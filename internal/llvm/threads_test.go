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
		if out, err := exec.Command(bin).CombinedOutput(); err != nil {
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
	cmd := exec.Command(bin)
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
	ir, err := xotool.LLVMIRRace(dir)
	if err != nil {
		t.Fatal(err)
	}
	ll := filepath.Join(t.TempDir(), "prog.ll")
	if err := os.WriteFile(ll, ir, 0o644); err != nil {
		t.Fatal(err)
	}
	t1 := time.Now()
	bin := tsanBuild(t, []string{"-Wno-override-module"}, []string{ll})
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
			ir, err := xotool.LLVMIRRace(dir)
			if err != nil {
				t.Fatal(err)
			}
			ll := filepath.Join(t.TempDir(), "prog.ll")
			if err := os.WriteFile(ll, ir, 0o644); err != nil {
				t.Fatal(err)
			}
			bin := tsanBuild(t, []string{"-Wno-override-module"}, []string{ll})
			for _, threads := range []string{"1", "2", ""} {
				cmd := exec.Command(bin)
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
		cmd := exec.CommandContext(ctx, res.Binaries[xotool.Host()])
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
	bin := tsanBuild(t, []string{"-Wno-override-module"}, []string{ll})
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), tsanOptions())
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "01010101" {
		t.Fatalf("got %q (%v), want 01010101 (the last byte: the heap mapped through the C library)", out, err)
	}
}
