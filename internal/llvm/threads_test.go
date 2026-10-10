package llvm_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/llvm"
)

// Thread safety of the runtime and of shared reference counts (decision
// 0058) and of the task runtime (decision 0068). Threads come from Xo
// programs that use tasks (TestGoldenTasks) and from C:
// std/llrt/testdata/rc_threads.c hands shared values to pthreads,
// std/llrt/testdata/tasks.c drives the task runtime directly, and
// -DXO_RT_TEST_THREADS runs whole programs on several threads at once.
// All run under ThreadSanitizer when clang supports it on the host.

var rtDir = filepath.Join("..", "..", "std", "llrt")

// tsanBuild compiles srcs with ThreadSanitizer and extra flags, or skips
// the test when the host's clang cannot. Where ThreadSanitizer compiles
// but cannot run, it compiles without it.
func tsanBuild(t *testing.T, flags, srcs []string) string {
	t.Helper()
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	bin := filepath.Join(t.TempDir(), "prog")
	san := []string{"-fsanitize=thread"}
	if why := sanitizerRuns("-fsanitize=thread"); why != "" {
		// it compiles but cannot start (x86-64 emulation): still run the
		// program, without the sanitizer
		t.Logf("ThreadSanitizer does not run here, testing without it: %s", why)
		san = nil
	}
	err := build.CompileC(append(append([]string{"-O1", "-g"}, san...), flags...), srcs, bin)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported option") || strings.Contains(err.Error(), "sanitizer") {
			t.Skipf("no ThreadSanitizer: %v", err)
		}
		t.Fatal(err)
	}
	return bin
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
// host skips the sanitizer tests instead of failing them.
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
		bin := filepath.Join(dir, "probe")
		if err := os.WriteFile(src, []byte("int main(void) { return 0; }\n"), 0o644); err != nil {
			p.why = err.Error()
			return
		}
		if err := build.CompileC([]string{"-O1", flag}, []string{src}, bin); err != nil {
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

// runTSan runs bin and fails on a nonzero exit or a sanitizer report.
func runTSan(t *testing.T, bin, dir string, env ...string) string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "TSAN_OPTIONS=halt_on_error=1"), env...)
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

// TestRuntimeThreads runs the C stress test of shared counts, the claim
// of in place updates, cross thread frees, the share walkers, and
// immortal literals: under ThreadSanitizer, then optimized with more
// iterations.
func TestRuntimeThreads(t *testing.T) {
	src := filepath.Join(rtDir, "testdata", "rc_threads.c")
	bin := tsanBuild(t, nil, []string{src})
	out := runTSan(t, bin, ".")
	for _, name := range []string{"string", "list", "claim", "append", "box", "queue", "literal"} {
		if !strings.Contains(out, name+": ok") {
			t.Errorf("test %s did not pass:\n%s", name, out)
		}
	}
	if testing.Short() {
		return
	}
	opt := filepath.Join(t.TempDir(), "opt")
	if err := build.CompileC([]string{"-O2", "-DITER=20000"}, []string{src}, opt); err != nil {
		t.Fatal(err)
	}
	runTSan(t, opt, ".")
}

// TestRuntimeTasks runs the C stress test of the task runtime (decision
// 0068): scopes with thousands of tasks, trees of nested scopes, awaits
// that run tasks not started yet, channels (buffered and not), canceled
// background tasks blocked in send and receive, child failure, mutexes
// and atomics. Under ThreadSanitizer with one worker and with three,
// then optimized with ten times the tasks; every allocation must be
// freed.
func TestRuntimeTasks(t *testing.T) {
	src := filepath.Join(rtDir, "testdata", "tasks.c")
	bin := tsanBuild(t, nil, []string{src})
	for _, threads := range []string{"1", "3"} {
		out := runTSan(t, bin, ".", "XO_THREADS="+threads)
		for _, name := range []string{"fanout", "tree", "channel", "cancel", "failure", "mutex"} {
			if !strings.Contains(out, name+": ok") {
				t.Errorf("XO_THREADS=%s: test %s did not pass:\n%s", threads, name, out)
			}
		}
	}
	if testing.Short() {
		return
	}
	opt := filepath.Join(t.TempDir(), "opt")
	if err := build.CompileC([]string{"-O2", "-DN=20000"}, []string{src}, opt); err != nil {
		t.Fatal(err)
	}
	runTSan(t, opt, ".")
}

// TestRuntimeEventLoop runs the C test of fibers and the event loop
// (decision 0069): thousands of sleepers on few workers, an echo of more
// data than the socket buffers hold, close and cancellation waking parked
// socket operations, a refused dial, and select's `after`. Under
// ThreadSanitizer with one worker and with three, then optimized; every
// allocation must be freed.
func TestRuntimeEventLoop(t *testing.T) {
	src := filepath.Join(rtDir, "testdata", "evloop.c")
	bin := tsanBuild(t, nil, []string{src})
	for _, threads := range []string{"1", "3"} {
		out := runTSan(t, bin, ".", "XO_THREADS="+threads)
		for _, name := range []string{"timers", "echo", "wakeups"} {
			if !strings.Contains(out, name+": ok") {
				t.Errorf("XO_THREADS=%s: test %s did not pass:\n%s", threads, name, out)
			}
		}
	}
	if testing.Short() {
		return
	}
	opt := filepath.Join(t.TempDir(), "opt")
	if err := build.CompileC([]string{"-O2"}, []string{src}, opt); err != nil {
		t.Fatal(err)
	}
	runTSan(t, opt, ".")
}

// deepRecursion lists golden programs that recurse hundreds of thousands
// of calls deep: ThreadSanitizer crashes on them even on one thread (its
// own limit, checked 2026-10-08), so they run on four threads without it.
var deepRecursion = map[string]bool{"recursion": true, "rc_closures": true}

// TestGoldenThreads builds golden programs with share probes (every heap
// value marked shared) and runs each on four threads at once
// (-DXO_RT_TEST_THREADS=4) under ThreadSanitizer: the per thread heaps,
// the deferred work lists, the output lock, the atomic count paths of
// generated code, and literals shared by every thread (never written)
// must not race, and every allocation must be freed.
func TestGoldenThreads(t *testing.T) {
	if testing.Short() {
		t.Skip("builds golden programs with ThreadSanitizer")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	rt := filepath.Join(rtDir, "xo_rt.c")
	ran := 0
	for _, dir := range dirs {
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(want, []byte("exit 0\n")) {
			continue // a fault exits the process from one thread
		}
		dir := dir
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			res, ok, err := build.CheckNative([]string{dir}, false)
			if err != nil || !ok {
				t.Fatalf("check: %v", err)
			}
			abs, _ := filepath.Abs(dir)
			prog, err := llvm.Generate(res, llvm.Options{LineRoot: abs, ShareProbes: true})
			if err != nil {
				t.Fatal(err)
			}
			ll := filepath.Join(t.TempDir(), "prog.ll")
			if err := os.WriteFile(ll, prog.IR, 0o644); err != nil {
				t.Fatal(err)
			}
			flags := append([]string{"-DXO_RT_TEST_THREADS=4", "-Wno-override-module"}, build.IOFlags(prog.IR)...)
			var bin string
			if deepRecursion[filepath.Base(dir)] {
				bin = filepath.Join(t.TempDir(), "prog")
				if err := build.CompileC(append([]string{"-O1"}, flags...), []string{ll, rt}, bin); err != nil {
					t.Fatal(err)
				}
			} else {
				bin = tsanBuild(t, flags, []string{ll, rt})
			}
			out := runTSan(t, bin, dir, "XO_LEAKCHECK=1")
			if m := leakRE.FindStringSubmatch(out); m == nil || m[1] != "0" {
				t.Errorf("leak check: %q", m)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no golden program exits normally")
	}
}

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
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	rt := filepath.Join(rtDir, "xo_rt.c")
	ran := 0
	for _, dir := range dirs {
		if !usesTasks(t, dir) {
			continue
		}
		dir := dir
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			res, ok, err := build.CheckNative([]string{dir}, false)
			if err != nil || !ok {
				t.Fatalf("check: %v", err)
			}
			abs, _ := filepath.Abs(dir)
			prog, err := llvm.Generate(res, llvm.Options{LineRoot: abs})
			if err != nil {
				t.Fatal(err)
			}
			ll := filepath.Join(t.TempDir(), "prog.ll")
			if err := os.WriteFile(ll, prog.IR, 0o644); err != nil {
				t.Fatal(err)
			}
			bin := tsanBuild(t, append([]string{"-Wno-override-module"}, build.IOFlags(prog.IR)...), []string{ll, rt})
			for _, threads := range []string{"1", "2", ""} {
				cmd := exec.Command(bin)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "TSAN_OPTIONS=halt_on_error=1", "XO_LEAKCHECK=1", "XO_THREADS="+threads)
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
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := filepath.Join("..", "gen", "testdata", "golden", "deadlock_real")
	backends := []string{build.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, build.BackendArm64)
	}
	for _, bk := range backends {
		res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: bk})
		if err != nil || res.Failed {
			t.Fatalf("%s: build: %v", bk, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		cmd := exec.CommandContext(ctx, res.Binaries[build.Host()])
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
