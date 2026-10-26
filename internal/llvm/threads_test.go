package llvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// Thread safety of the runtime and of shared reference counts (decision
// 0058) and of the task runtime (decision 0068). Threads come from Xo
// programs that use tasks (TestGoldenTasks) and from the runtime's stress
// tests, Xo programs since decision 0241 (testdata/rt: tasks, rc_threads,
// alloc_threads; std/internal's rt_probe for what the language does not
// show). All run under
// ThreadSanitizer when clang supports it on the host.

var rtDir = filepath.Join("..", "..", "std", "llrt")

// tsanBuild compiles srcs with ThreadSanitizer and extra flags, or skips
// the test when the host's clang cannot. Where ThreadSanitizer compiles
// but cannot run, it compiles without it.

func tsanBuild(t *testing.T, flags, srcs []string) string {
	t.Helper()
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	bin := exe(filepath.Join(t.TempDir(), "prog"))
	// without the instrumentation of function entry and exit: it gives
	// every function that calls a cleanup landing pad that resumes the
	// unwind (__tsan_func_exit, then _Unwind_Resume), which the Xo fault
	// unwinder cannot resume (decision 0221 stage 3,
	// gotchas/tsan-func-exit-pads.md); reports lose the stack above the
	// racing frame
	san := []string{"-fsanitize=thread", "-mllvm", "-tsan-instrument-func-entry-exit=0"}
	if why := sanitizerRuns("-fsanitize=thread"); why != "" {
		// it compiles but cannot start (x86-64 emulation): still run the
		// program, without the sanitizer
		t.Logf("ThreadSanitizer does not run here, testing without it: %s", why)
		san = nil
	}
	err := xotool.CompileC(append(append([]string{"-O1", "-g"}, san...), flags...), srcs, bin)
	if err != nil && san != nil && runtime.GOOS == "windows" && strings.Contains(err.Error(), "unsupported option") {
		// clang has no ThreadSanitizer for the MSVC target (decision
		// 0197): the threads still run, without it
		t.Logf("no ThreadSanitizer for this target, testing without it")
		err = xotool.CompileC(append([]string{"-O1", "-g"}, flags...), srcs, bin)
	}
	if err != nil {
		if strings.Contains(err.Error(), "unsupported option") || strings.Contains(err.Error(), "sanitizer") {
			t.Skipf("no ThreadSanitizer: %v", err)
		}
		t.Fatal(err)
	}
	return bin
}

// tsanBuildRuntime is tsanBuild for the runtime's own race tests
// (decision 0273 mode 1): the binary also defines xo_tsan_runtime, which
// tells the runtime so (sanhook.xo user_mode: fibers synchronize at a
// switch, Xo's heap); its IR comes from xotool.LLVMIRRaceRuntime (the
// runtime instrumented) or LLVMIR.
func tsanBuildRuntime(t *testing.T, flags, srcs []string) string {
	t.Helper()
	c := filepath.Join(t.TempDir(), "xo_tsan_runtime.c")
	if err := os.WriteFile(c, []byte("int xo_tsan_runtime = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return tsanBuild(t, flags, append(append([]string{}, srcs...), c))
}

// asanFlags returns flags, without AddressSanitizer where it compiles but
// cannot start (sanitizerRuns): "-fsanitize=address,undefined" keeps
// UndefinedBehaviorSanitizer, "-fsanitize=address" is dropped. The
// program still runs and its output is still compared.
func asanFlags(flags ...string) []string {
	if sanitizerRuns("-fsanitize=address") == "" {
		return flags
	}
	var out []string
	for _, f := range flags {
		switch f {
		case "-fsanitize=address":
			continue
		case "-fsanitize=address,undefined":
			f = "-fsanitize=undefined"
		}
		out = append(out, f)
	}
	return out
}

var sanProbes sync.Map // sanitizer flag -> *sanProbe

type sanProbe struct {
	once sync.Once
	why  string
}

// sanitizerRuns builds and runs an empty C program with a sanitizer flag
// and returns why it built but did not run, or "". ThreadSanitizer compiles
// but cannot start under x86-64 emulation on an arm64 host ("memory
// layout is incompatible", gotchas/tsan-under-x86-emulation.md); such a
// host skips the sanitizer tests instead of failing them. On the Intel
// Mac (macOS 26.5.2, Apple clang 17) AddressSanitizer and
// ThreadSanitizer binaries die with SIGILL in __pthread_init before main
// (gotchas/darwin-amd64-sanitizers-sigill.md).
func sanitizerRuns(flag string) string {
	v, _ := sanProbes.LoadOrStore(flag, &sanProbe{})
	p := v.(*sanProbe)
	p.once.Do(func() {
		dir, err := os.MkdirTemp("", "xo-san")
		if err != nil {
			p.why = err.Error()
			return
		}
		defer os.RemoveAll(dir)
		src := filepath.Join(dir, "probe.c")
		bin := exe(filepath.Join(dir, "probe"))
		if err := os.WriteFile(src, []byte("int main(void) { return 0; }\n"), 0o644); err != nil {
			p.why = err.Error()
			return
		}
		if err := xotool.CompileC([]string{"-O1", flag}, []string{src}, bin); err != nil {
			return // no such sanitizer: the real build reports it
		}
		if out, err := testtmp.Guard(exec.Command(bin)).CombinedOutput(); err != nil {
			text := strings.TrimSpace(string(out))
			if i := strings.IndexByte(text, '\n'); i >= 0 {
				text = text[:i]
			}
			p.why = fmt.Sprintf("%v: %s", err, text)
		}
	})
	return p.why
}

// tsanOptions is the ThreadSanitizer setting of every run: stop at the
// first report, and no sleep at exit. ThreadSanitizer sleeps 1 s in
// atexit by default (atexit_sleep_ms) to catch races of threads still
// running then; a run took 2.5 s instead of 0.25 s on 2026-10-09
// (gotchas/race-binaries-sleep-at-exit.md). XO_TSAN_ATEXIT_SLEEP=ms
// (bench/nightly.sh: 1000) restores the sleep.
func tsanOptions() string {
	ms := os.Getenv("XO_TSAN_ATEXIT_SLEEP")
	if ms == "" {
		ms = "0"
	}
	return "TSAN_OPTIONS=halt_on_error=1:atexit_sleep_ms=" + ms
}

// runTSan runs bin and fails on a nonzero exit or a sanitizer report.
func runTSan(t *testing.T, bin, dir string, env ...string) string {
	t.Helper()
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), tsanOptions()), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := out.String()
	if err != nil || strings.Contains(text, "ThreadSanitizer") {
		if len(text) > 4000 {
			text = text[:4000]
		}
		t.Fatalf("%s: %v\n%s", filepath.Base(bin), err, text)
	}
	return text
}

// TestRuntimeThreads runs the stress test of shared counts (decision
// 0058; rc.xo), an Xo program since decision 0241 (testdata/rt/rc_threads;
// it was std/llrt/testdata/rc_threads.c): shared strings, lists, and a
// chain of 10,000 cells captured by eight tasks running at once, copies
// on write, the claim of in place updates, appends, frees crossing
// threads through a channel, immortal literals, and a model of random
// retains, releases, claims, and marks. Under ThreadSanitizer with one
// worker, three, and eight, then optimized with ten times the
// iterations.
func TestRuntimeThreads(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	runRuntimeXo(t, filepath.Join("testdata", "rt", "rc_threads"), "ITER=20000",
		[]string{"string", "list", "claim", "append", "box", "queue", "literal", "model"})
}

// TestRuntimeAlloc runs the stress test of the native heap (decision 0207
// stage 3, alloc.xo), an Xo program since decision 0241
// (testdata/rt/alloc_threads; it was std/llrt/testdata/alloc_threads.c):
// random sizes over every class and the system's blocks, resizes, frees
// on other threads through mailboxes, and waves of blocks freed by other
// threads, every block's contents checked. Under ThreadSanitizer with one
// worker, three, and eight, then optimized with ten times the
// iterations.
func TestRuntimeAlloc(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	runRuntimeXo(t, filepath.Join("testdata", "rt", "alloc_threads"), "ITER=30000",
		[]string{"solo", "threads", "waves"})
}

// TestRuntimeTasks runs the stress test of the task runtime (decision
// 0068), an Xo program since decision 0241 (testdata/rt/tasks; it was
// std/llrt/testdata/tasks.c): scopes with thousands of tasks, trees of
// nested scopes, channels (buffered and not), canceled background tasks
// blocked in send and receive, child failure, mutexes and atomics, the
// mutex's phase fair handoff, the pruning of a long lived scope, and
// magazines. Under ThreadSanitizer with one worker, three, and eight,
// then optimized with ten times the tasks; every allocation of each test
// must be freed.
func TestRuntimeTasks(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	runRuntimeXo(t, filepath.Join("testdata", "rt", "tasks"), "XO_TASKS_N=20000",
		[]string{"fanout", "tree", "channel", "cancel", "failure", "mutex", "handoff", "prune", "magazines"})
}

// runRuntimeXo runs a runtime stress test written in Xo (decision 0241):
// the program in dir, its LLVM IR built with ThreadSanitizer and run with
// one worker, three, and eight and the leak check on, each run printing
// "<test>: ok" for every name and ending with no allocation live; then,
// without -short, built optimized by xo and run once with more work
// (more, an environment entry) on eight workers.
func runRuntimeXo(t *testing.T, dir, more string, names []string) {
	t.Helper()
	t0 := time.Now()
	// --race marks every function sanitize_thread, the runtime's too:
	// clang's ThreadSanitizer pass instruments only marked functions of IR
	// it is given (gotchas/tsan-needs-sanitize-thread-mark.md)
	ir, err := xotool.LLVMIRRaceRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	ll := filepath.Join(t.TempDir(), "prog.ll")
	if err := os.WriteFile(ll, ir, 0o644); err != nil {
		t.Fatal(err)
	}
	t1 := time.Now()
	bin := tsanBuildRuntime(t, []string{"-Wno-override-module"}, []string{ll})
	t.Logf("LLVM IR %v, ThreadSanitizer build %v", t1.Sub(t0), time.Since(t1))
	check := func(out, what string) {
		t.Helper()
		for _, name := range names {
			if !strings.Contains(out, name+": ok") {
				t.Errorf("%s: test %s did not pass:\n%s", what, name, out)
			}
		}
		if !strings.Contains(out, "xo leakcheck: 0 live") {
			t.Errorf("%s: allocations live at exit:\n%s", what, out)
		}
	}
	for _, threads := range []string{"1", "3", "8"} {
		t2 := time.Now()
		check(runTSan(t, bin, dir, "XO_THREADS="+threads, "XO_LEAKCHECK=1"), "XO_THREADS="+threads)
		t.Logf("XO_THREADS=%s: %v", threads, time.Since(t2))
	}
	if testing.Short() {
		return
	}
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	env := []string{"XO_THREADS=8", "XO_LEAKCHECK=1"}
	if more != "" {
		env = append(env, more)
	}
	check(runTSan(t, res.Binaries[xotool.Host()], dir, env...), "optimized "+more)
}

// TestRuntimeEventLoop runs the test of fibers and the event loop
// (decision 0069), an Xo program since decision 0241 (testdata/rt/evloop;
// it was std/llrt/testdata/evloop.c and rawpoll.c): thousands of sleepers
// on few workers, an echo of more data than the socket buffers hold, close
// and cancellation waking parked socket operations, a refused dial,
// select's `after`, descriptors registered while the poller waits, and a
// pipe a child process inherited (Linux). Under ThreadSanitizer with one
// worker, three, and eight, then optimized; every allocation of each test
// must be freed.
func TestRuntimeEventLoop(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	runRuntimeXo(t, filepath.Join("testdata", "rt", "evloop"), "",
		[]string{"timers", "echo", "wakeups", "registrations", "inherited"})
}

// deepRecursion lists golden programs that recurse hundreds of thousands
// of calls deep: ThreadSanitizer crashes on them even on one thread (its
// own limit, checked 2026-10-08), so they run on four threads without it.
var deepRecursion = map[string]bool{"recursion": true, "rc_closures": true}

// fiberBound is the most tasks a golden program parks at once (each
// holds a fiber stack). Four copies of it run at once in
// TestGoldenThreads, so its fibers at once are at most four times that,
// plus one running per worker thread, each worker's cache of 8, and the
// shared cache of 64 (std/llrt/xo_rt.c: XO_FCACHE, XO_FIBER_CACHE).
// conc_stress parks its 2000 senders on a full channel; the bound is its
// memory need: under ThreadSanitizer on Linux, whose arm64 programs have
// 31.8 GiB to map in (about 24 GiB of it left for fiber stacks), the
// stacks are 2 MiB, so the bound on 32 CPUs (8352 fibers) reserves 16.3
// GiB (gotchas/linux-tsan-fiber-stacks.md). More than the bound would be
// a fiber leak.
var fiberBound = map[string]int{"conc_stress": 2000}

var fiberPeakRE = regexp.MustCompile(`fiber_peak=(\d+)`)

// TestGoldenThreads built every golden program with share probes (every
// heap value marked shared as it is defined, decision 0058) under
// ThreadSanitizer; the probes are a pass of the Go SSA builder that
// compiler/ssa does not have, so it went with decision 0215.

// usesTasks reports whether the golden program in dir uses scopes,
// channels, mutexes, or atomics.
func usesTasks(t *testing.T, dir string) bool {
	src, err := os.ReadFile(filepath.Join(dir, "main.xo"))
	if err != nil {
		t.Fatal(err)
	}
	// a `use c` program (use_c_add) links its C adapter and sources,
	// which the IR alone TestGoldenTasks builds from does not carry
	// (undefined _xo_c_... symbols); TestGolden runs it with its tasks
	if bytes.HasPrefix(src, []byte("use c ")) || bytes.Contains(src, []byte("\nuse c ")) {
		return false
	}
	for _, w := range []string{"scope ", "Chan[", "Mutex[", "Atomic["} {
		if bytes.Contains(src, []byte(w)) {
			return true
		}
	}
	return false
}

// TestGoldenTasks builds the golden programs that use tasks with
// ThreadSanitizer (no share probes: only the crossings the lowering
// marks) and runs each with one worker thread, with two, and with one
// per CPU: the output must be want.txt every time, with no sanitizer
// report and no allocation live at exit.
func TestGoldenTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds golden programs with ThreadSanitizer")
	}
	t.Parallel() // builds nothing in the shared build directory
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	ran := 0
	for _, dir := range dirs {
		if !usesTasks(t, dir) {
			continue
		}
		dir := dir
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(wantFile(dir))
			if err != nil {
				t.Fatal(err)
			}
			// every function checked, the runtime's too (--race)
			ir, err := xotool.LLVMIRRaceRuntime(dir)
			if err != nil {
				t.Fatal(err)
			}
			ll := filepath.Join(t.TempDir(), "prog.ll")
			if err := os.WriteFile(ll, ir, 0o644); err != nil {
				t.Fatal(err)
			}
			bin := tsanBuildRuntime(t, []string{"-Wno-override-module"}, []string{ll})
			for _, threads := range []string{"1", "2", ""} {
				cmd := testtmp.Guard(exec.Command(bin))
				cmd.Dir = dir
				// the golden's env.txt too (http_conn_fault's probe)
				cmd.Env = append(append(os.Environ(), progEnv(dir)...), tsanOptions(), "XO_LEAKCHECK=1", "XO_THREADS="+threads)
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				err := cmd.Run()
				code := 0
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					code = ee.ExitCode()
				} else if err != nil {
					t.Fatalf("run: %v", err)
				}
				if strings.Contains(stderr.String(), "ThreadSanitizer") {
					text := stderr.String()
					if len(text) > 4000 {
						text = text[:4000]
					}
					t.Fatalf("XO_THREADS=%q: %s", threads, text)
				}
				var keep []string
				for _, l := range strings.Split(stderr.String(), "\n") {
					if !strings.HasPrefix(l, "xo leakcheck:") {
						keep = append(keep, l)
					}
				}
				got := runResult{exit: code, stdout: stdout.String(), stderr: stableStderr(strings.Join(keep, "\n"))}
				if got.String() != string(want) {
					t.Errorf("XO_THREADS=%q: output differs from want.txt\n--- want\n%s\n--- got\n%s", threads, want, got)
				}
				if code == 0 {
					if m := leakRE.FindStringSubmatch(stderr.String()); m == nil || m[1] != "0" {
						t.Errorf("XO_THREADS=%q: leak check: %q", threads, m)
					}
				}
			}
		})
	}
	if ran == 0 {
		t.Fatal("no golden program uses tasks")
	}
}

// TestDeadlock builds internal/gen's deadlock_real (read only) with the
// native backends: when every task is blocked the runtime faults with
// XO-F001 (exit 70) instead of hanging. The native report has the first
// line of the Go backend's, without the location and the list of
// blocked tasks.
func TestDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := filepath.Join("..", "gen", "testdata", "golden", "deadlock_real")
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, bk := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: bk})
		if err != nil || res.Failed {
			t.Fatalf("%s: build: %v", bk, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		cmd := testtmp.Guard(exec.CommandContext(ctx, res.Binaries[xotool.Host()]))
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err = cmd.Run()
		cancel()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 70 {
			t.Fatalf("%s: want exit status 70, got %v\n%s", bk, err, stderr.String())
		}
		if stdout.String() != "start\n" || !strings.HasPrefix(stderr.String(), "fault: XO-F001: deadlock: every task is blocked\n") {
			t.Errorf("%s: stdout %q stderr %q", bk, stdout.String(), stderr.String())
		}
	}
}

// TestTSanSameThreadRace: a race planted between four tasks
// (testdata/tsan_race) is reported by a --race build with one worker
// thread between two task fibers (ThreadSanitizer threads with tid 0):
// the runtime switches fibers without synchronizing them (sanhook.xo
// TSAN_NO_SYNC); with __tsan_switch_to_fiber's flag 0 every switch
// ordered the fibers of a thread and only a race with the worker's own
// context was seen (gotchas/tsan-fiber-switch-sync.md).
func TestTSanSameThreadRace(t *testing.T) {
	sameThreadRace(t, raceBuild(t, "tsan_race", xotool.BackendLLVM))
}

// TestTSanSameThreadRaceArm64 is TestTSanSameThreadRace on the arm64
// backend's --race (decision 0273 part 2).
func TestTSanSameThreadRaceArm64(t *testing.T) {
	sameThreadRace(t, raceBuild(t, "tsan_race", xotool.BackendArm64))
}

// TestRaceStacksArm64: a report's stacks (testdata/race_stacks): both
// accesses show touch's call site, the task's its closure, and main's
// stack holds no frame the 200 isolated faults left (each would repeat
// main's line of the isolate call).
func TestRaceStacksArm64(t *testing.T) {
	cmd := testtmp.Guard(exec.Command(raceBuild(t, "race_stacks", xotool.BackendArm64)))
	cmd.Env = append(os.Environ(), "TSAN_OPTIONS=halt_on_error=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	text := stderr.String()
	if !strings.Contains(stdout.String(), "faults 200") {
		t.Fatalf("stdout %q, want faults 200:\n%s", stdout.String(), text)
	}
	rep, _, _ := strings.Cut(text, "Location is")
	for _, want := range []string{"xo_f_touch main.xo:", "xo_f_main_2efn", "xo_f_main main.xo:"} {
		if !strings.Contains(rep, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
	// the task's creation stack ends at main's spawn (cabi.xo
	// task_spawn_fn's return address, sanhook.xo task_new)
	if _, created, ok := strings.Cut(text, "created by thread"); !ok || !strings.Contains(strings.SplitN(created, "\n\n", 2)[0], "xo_f_main main.xo:") {
		t.Errorf("the task's creation stack lacks main's spawn site:\n%s", text)
	}
	if n := strings.Count(rep, "xo_f_main main.xo:"); n > 2 {
		t.Errorf("main has %d frames in the report (frames the isolated faults left):\n%s", n, text)
	}
}

func sameThreadRace(t *testing.T, bin string) {
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Env = append(os.Environ(), "TSAN_OPTIONS=halt_on_error=0", "XO_THREADS=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	text := stderr.String()
	// the fibers: "Thread T4 (tid=0, running) created by thread T2"
	fibers := map[string]bool{}
	for _, m := range regexp.MustCompile(`Thread (T\d+) \(tid=0,`).FindAllStringSubmatch(text, -1) {
		fibers[m[1]] = true
	}
	for _, rep := range strings.Split(text, "WARNING: ThreadSanitizer: data race")[1:] {
		ts := regexp.MustCompile(`by thread (T\d+)`).FindAllStringSubmatch(rep, 2)
		if len(ts) == 2 && ts[0][1] != ts[1][1] && fibers[ts[0][1]] && fibers[ts[1][1]] {
			return
		}
	}
	t.Errorf("no data race reported between two task fibers (stdout %q):\n%s", stdout.String(), text)
}

// TestRaceSyncTable: --race (decision 0273 mode 2) on each
// synchronization a program has (testdata/race_sync): used correctly, no
// race may be reported; with it left out, the race must be. With one
// worker thread, where every task is a fiber of it, and with the
// default. The table is what the runtime's explicit acquire and release
// (tasks.xo, chan.xo, selects.xo, mutex.xo) are trusted by.
func TestRaceSyncTable(t *testing.T) {
	raceSyncTable(t, raceBuild(t, "race_sync", xotool.BackendLLVM))
}

// TestRaceSyncTableArm64 is TestRaceSyncTable on the arm64 backend's
// --race (decision 0273 part 2: compiler/arm64 race.xo instruments the
// program's loads, stores, and atomics; the runtime is mode 2's).
func TestRaceSyncTableArm64(t *testing.T) {
	raceSyncTable(t, raceBuild(t, "race_sync", xotool.BackendArm64))
}

// raceBuild builds testdata/name with --race (decision 0273 mode 2) on
// backend: llvm (its IR through clang's -fsanitize=thread) or arm64
// (xo build on darwin; elsewhere its assembly, already instrumented,
// linked with clang's ThreadSanitizer runtime).
func raceBuild(t *testing.T, name, backend string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a program with ThreadSanitizer")
	}
	if why := sanitizerRuns("-fsanitize=thread"); why != "" {
		t.Skipf("ThreadSanitizer does not run here: %s", why)
	}
	dir := filepath.Join("testdata", name)
	if backend == xotool.BackendLLVM {
		ir, err := xotool.LLVMIRRace(dir)
		if err != nil {
			t.Fatal(err)
		}
		ll := filepath.Join(t.TempDir(), "prog.ll")
		if err := os.WriteFile(ll, ir, 0o644); err != nil {
			t.Fatal(err)
		}
		return tsanBuild(t, []string{"-Wno-override-module"}, []string{ll})
	}
	if !xotool.Arm64Host() {
		t.Skip("the arm64 backend's --race runs on an arm64 host")
	}
	if runtime.GOOS == "darwin" {
		// xo build links it (decision 0273 part 3: Xo's linker, the
		// runtime a dylib), with clang's runtime until one is shipped
		bin := filepath.Join(t.TempDir(), "prog")
		var env []string
		if os.Getenv("XO_RACE_RUNTIME") == "" {
			cl, err := xotool.Clang()
			if err != nil {
				t.Skip("clang not found")
			}
			out, err := exec.Command(cl, "-print-file-name=lib/darwin/libclang_rt.tsan_osx_dynamic.dylib").Output()
			if err != nil {
				t.Skipf("no ThreadSanitizer runtime: %v", err)
			}
			env = append(env, "XO_RACE_RUNTIME="+strings.TrimSpace(string(out)))
		}
		r, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Race: true, Out: bin, Env: env})
		if err != nil {
			t.Fatal(err)
		}
		if r.Failed {
			t.Fatalf("xo build --race --backend=arm64 failed:\n%s", r.Output)
		}
		return bin
	}
	asm, err := xotool.Arm64Race(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "prog.s")
	if err := os.WriteFile(src, asm, 0o644); err != nil {
		t.Fatal(err)
	}
	return tsanBuild(t, nil, []string{src})
}

func raceSyncTable(t *testing.T, bin string) {
	cases := []string{"spawn", "join", "scope", "chan", "chan_buffered", "select", "mutex", "mutex_loop", "await_done", "atomic", "atomic_add", "lazy_const", "map_grow"}
	// cases with no unsynchronized form: only "no race" is checked
	syncOnly := map[string]bool{"lazy_const": true, "mutex_loop": true}
	for _, c := range cases {
		for _, threads := range []string{"1", ""} {
			for _, sync := range []string{"1", "0"} {
				if sync == "0" && syncOnly[c] {
					continue
				}
				cmd := testtmp.Guard(exec.Command(bin))
				cmd.Env = append(os.Environ(), "TSAN_OPTIONS=halt_on_error=0", "XO_THREADS="+threads, "XO_RACE_CASE="+c, "XO_RACE_SYNC="+sync)
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				testtmp.Lifeline(t, cmd, time.Minute)
				_ = cmd.Run()
				raced := strings.Contains(stderr.String(), "WARNING: ThreadSanitizer: data race")
				if !strings.Contains(stdout.String(), "done") {
					t.Errorf("%s threads=%q sync=%s: did not finish (stdout %q):\n%s", c, threads, sync, stdout.String(), stderr.String())
				} else if sync == "1" && raced {
					t.Errorf("%s threads=%q: a race reported although synchronized:\n%s", c, threads, stderr.String())
				} else if sync == "0" && !raced {
					t.Errorf("%s threads=%q: no race reported without the synchronization", c, threads)
				}
			}
		}
	}
}

// TestSysCTSan: rt.sym_present of a weak reference (decision 0232,
// std/native/sys_c.xo) is true when ThreadSanitizer's runtime is linked
// (testdata/sysc_tsan prints 010101; the sys_c golden, without it,
// 0101 and a 00 the goldens do not show), and the heap is then mapped
// through the C library, where ThreadSanitizer's mmap interceptor sees
// it (heapos.xo map counts those mappings: a fourth byte 01).
func TestSysCTSan(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with ThreadSanitizer")
	}
	if why := sanitizerRuns("-fsanitize=thread"); why != "" {
		t.Skipf("ThreadSanitizer does not run here: %s", why)
	}
	ir, err := xotool.LLVMIR(filepath.Join("testdata", "sysc_tsan"))
	if err != nil {
		t.Fatal(err)
	}
	ll := filepath.Join(t.TempDir(), "prog.ll")
	if err := os.WriteFile(ll, ir, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := tsanBuildRuntime(t, []string{"-Wno-override-module"}, []string{ll})
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Env = append(os.Environ(), tsanOptions())
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "01010101" {
		t.Fatalf("got %q (%v), want 01010101 (the last byte: the heap mapped through the C library)", out, err)
	}
}

// TestRaceDefaultNative: --race with no --backend builds with the native
// backend where a ThreadSanitizer runtime is found (decision 0273's
// 2026-10-22 addendum), so it needs no clang: with clang off PATH and the
// runtime named by XO_RACE_RUNTIME, race_stacks builds and its race is
// reported. With no runtime for the target (none in the checkout) it
// falls back to LLVM, whose missing clang is an error saying so.
func TestRaceDefaultNative(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with ThreadSanitizer")
	}
	if runtime.GOOS != "darwin" || !xotool.Arm64Host() {
		t.Skip("darwin/arm64: the runtime is clang's dylib")
	}
	cl, err := xotool.Clang()
	if err != nil {
		t.Skip("clang not found (it finds the runtime)")
	}
	out, err := exec.Command(cl, "-print-file-name=lib/darwin/libclang_rt.tsan_osx_dynamic.dylib").Output()
	rt := strings.TrimSpace(string(out))
	if err != nil || !filepath.IsAbs(rt) {
		t.Skipf("no ThreadSanitizer runtime: %v", err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "path")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	// the environment without PATH, XO_CLANG and XO_RACE_RUNTIME
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PATH=") && !strings.HasPrefix(e, "XO_CLANG=") && !strings.HasPrefix(e, "XO_RACE_RUNTIME=") && !strings.HasPrefix(e, "XO_STD=") {
			env = append(env, e)
		}
	}
	env = append(env, "PATH="+empty, "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	bin := filepath.Join(tmp, "prog")
	cmd := testtmp.Guard(exec.Command(xo, "build", "--race", "-o", bin, filepath.Join("testdata", "race_stacks")))
	cmd.Env = append(env, "XO_RACE_RUNTIME="+rt)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build --race with no clang on PATH: %v\n%s", err, b)
	}
	run := testtmp.Guard(exec.Command(bin))
	run.Env = append(os.Environ(), "TSAN_OPTIONS=halt_on_error=0")
	var stdout, stderr bytes.Buffer
	run.Stdout, run.Stderr = &stdout, &stderr
	_ = run.Run()
	if !strings.Contains(stderr.String(), "WARNING: ThreadSanitizer: data race") || !strings.Contains(stderr.String(), "xo_f_touch main.xo:") {
		t.Errorf("no race reported (stdout %q):\n%s", stdout.String(), stderr.String())
	}
	// no runtime for the target: LLVM, and no clang
	fb := testtmp.Guard(exec.Command(xo, "build", "--race", "-o", filepath.Join(tmp, "fb"), filepath.Join("testdata", "race_stacks")))
	fb.Env = env
	b, err := fb.CombinedOutput()
	// the targets come from race_files, so a shipped one cannot go missing
	if err == nil || !strings.Contains(string(b), "no native ThreadSanitizer runtime ships for darwin/arm64") || !strings.Contains(string(b), "darwin/arm64, darwin/amd64, linux/arm64, linux/amd64 and windows/amd64 have one") {
		t.Errorf("xo build --race with no runtime and no clang: %v, want the no native runtime error:\n%s", err, b)
	}
}

// TestRaceBinaryOutlivesToolchain: a darwin --race binary built by Xo's
// Mach-O linker still runs after the toolchain's race runtime directory
// is renamed (field report on 0.26.0, 2026-10-23: it loaded the dylib by
// the toolchain's absolute path). The link loads
// @rpath/libclang_rt.tsan_osx_dynamic.dylib with the run path
// @executable_path, then the runtime's directory, and puts a copy of the
// runtime beside the binary.
func TestRaceBinaryOutlivesToolchain(t *testing.T) {
	if runtime.GOOS != "darwin" || !xotool.Arm64Host() {
		t.Skip("darwin/arm64's Mach-O race link")
	}
	if testing.Short() {
		t.Skip("builds a program with ThreadSanitizer")
	}
	if why := sanitizerRuns("-fsanitize=thread"); why != "" {
		t.Skipf("ThreadSanitizer does not run here: %s", why)
	}
	src := os.Getenv("XO_RACE_RUNTIME")
	if src == "" {
		cl, err := xotool.Clang()
		if err != nil {
			t.Skip("clang not found")
		}
		out, err := exec.Command(cl, "-print-file-name=lib/darwin/libclang_rt.tsan_osx_dynamic.dylib").Output()
		if err != nil {
			t.Skipf("no ThreadSanitizer runtime: %v", err)
		}
		src = strings.TrimSpace(string(out))
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("no ThreadSanitizer runtime: %v", err)
	}
	// the native backends link it with Xo's linker; LLVM's clang links
	// xo's runtime too, not its own (Apple clang 17's stopped at SIGILL
	// on an Intel Mac, field report on 0.26.0)
	for _, backend := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		t.Run(backend, func(t *testing.T) {
			tmp := t.TempDir()
			rt := filepath.Join(tmp, "toolchain", "race", filepath.Base(src))
			if err := os.MkdirAll(filepath.Dir(rt), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(rt, data, 0o755); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(tmp, "out", "prog")
			if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
				t.Fatal(err)
			}
			r, err := xotool.Build([]string{filepath.Join("testdata", "race_sync")}, xotool.Options{Backend: backend, Race: true, Out: bin, Env: []string{"XO_RACE_RUNTIME=" + rt}})
			if err != nil {
				t.Fatal(err)
			}
			if r.Failed {
				t.Fatalf("xo build --race --backend=%s failed:\n%s", backend, r.Output)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(bin), filepath.Base(src))); err != nil {
				t.Fatalf("no copy of the runtime beside the binary: %v", err)
			}
			// the same program again into another directory: the binary
			// cache's link, which copies the runtime there too
			bin2 := filepath.Join(tmp, "out2", "prog")
			if err := os.MkdirAll(filepath.Dir(bin2), 0o755); err != nil {
				t.Fatal(err)
			}
			r, err = xotool.Build([]string{filepath.Join("testdata", "race_sync")}, xotool.Options{Backend: backend, Race: true, Out: bin2, Env: []string{"XO_RACE_RUNTIME=" + rt}})
			if err != nil || r.Failed {
				t.Fatalf("second build: %v\n%s", err, r.Output)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(bin2), filepath.Base(src))); err != nil {
				t.Fatalf("a cached link left no copy of the runtime beside the binary: %v", err)
			}
			// the toolchain moves away
			if err := os.Rename(filepath.Join(tmp, "toolchain"), filepath.Join(tmp, "moved")); err != nil {
				t.Fatal(err)
			}
			cmd := testtmp.Guard(exec.Command(bin))
			cmd.Env = append(os.Environ(), "XO_RACE_CASE=spawn", "XO_RACE_SYNC=1")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("the race binary did not run after its toolchain moved: %v\n%s", err, out)
			}
			if strings.Contains(string(out), "WARNING: ThreadSanitizer") {
				t.Fatalf("a race reported in the synchronized case:\n%s", out)
			}
		})
	}
}
