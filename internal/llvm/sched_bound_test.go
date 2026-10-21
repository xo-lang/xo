package llvm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestDeferredWakeBound: a wake the scheduler defers (decision 0077's
// addendum: a channel's hand off, a ring's only spawn; mutexbusy checks a
// Mutex's grant, which main wakes directly for now)
// still runs its task while the waker keeps computing and never parks,
// because the monitor wakes an idle worker for it after about 1 ms.
// testdata/sched handoff (a sender), spawnbusy (a spawner), mutexbusy
// (a Mutex's holder), and rwbusy (a writer that granted a reader, whose
// grace a woken worker waits out) compute 200 ms after the wake and print the worst
// delay of 5 rounds at XO_THREADS=4; it must stay under 100 ms. Without
// the monitor the task waits the whole 200 ms; with it, about 2 ms on an
// idle machine, but its 1 ms ticks overslept by tens of ms on a Mac at
// load 25 (2026-10-16), so the bound tells broken from working, not fast
// from slow. At XO_THREADS=1 (spawnbusy and handoff) a root's deferral
// waits for the monitor too, with no 50 us wake: the same bound holds.
func TestDeferredWakeBound(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs two programs per native backend")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	worst := regexp.MustCompile(`worst (\d+) us \(busy\)`)
	for _, be := range backends {
		for _, run := range []struct{ prog, threads string }{{"handoff", "4"}, {"spawnbusy", "4"}, {"mutexbusy", "4"}, {"rwbusy", "4"}, {"handoff", "1"}, {"spawnbusy", "1"}} {
			prog := run.prog
			t.Run(be+"/"+prog+"/"+run.threads, func(t *testing.T) {
				if be == xotool.BackendLLVM {
					if _, err := xotool.Clang(); err != nil {
						t.Skip("clang not found")
					}
				}
				res, err := xotool.Build([]string{filepath.Join("testdata", "sched", prog)}, xotool.Options{Backend: be})
				if err != nil || res.Failed {
					t.Fatalf("build: %v %v", err, res)
				}
				cmd := exec.Command(res.Binaries[xotool.Host()])
				cmd.Env = append(os.Environ(), "XO_THREADS="+run.threads, "XO_LOCKCHECK=1")
				out, err := runCombined(cmd, 2*time.Minute)
				if err != nil {
					t.Fatalf("run: %v\n%s", err, out)
				}
				m := worst.FindStringSubmatch(out)
				if m == nil {
					t.Fatalf("output: %s", out)
				}
				us, _ := strconv.Atoi(m[1])
				if us >= 100000 {
					t.Fatalf("%s: the woken task waited %d us (bound 100 ms; the waker computed 200 ms)", prog, us)
				}
				t.Logf("%s: worst %d us", prog, us)
			})
		}
	}
}

// TestDeferWakeOff: deferred wakes (decision 0077's addendum) are off in
// a program with a `uses realtime` function (the compiler's
// rt.realtime_program at main's start) and with XO_DEFER_WAKE=0, so a
// real-time program keeps direct wakes' latency; a plain ping pong
// defers them. XO_RTSTATS counts defer.
func TestDeferWakeOff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs two programs")
	}
	be := xotool.BackendLLVM
	if runtime.GOARCH == "arm64" {
		be = xotool.BackendArm64
	} else if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	defers := regexp.MustCompile(`\bdefer=(\d+)`)
	run := func(prog string, env ...string) int {
		t.Helper()
		res, err := xotool.Build([]string{filepath.Join("testdata", "sched", prog)}, xotool.Options{Backend: be})
		if err != nil || res.Failed {
			t.Fatalf("build %s: %v %v", prog, err, res)
		}
		cmd := exec.Command(res.Binaries[xotool.Host()])
		cmd.Env = append(append(os.Environ(), "XO_THREADS=4", "XO_RTSTATS=1"), env...)
		out, err := runCombined(cmd, 2*time.Minute)
		if err != nil {
			t.Fatalf("%s: %v\n%s", prog, err, out)
		}
		m := defers.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("%s: no defer count:\n%s", prog, out)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if n := run("realtime"); n != 0 {
		t.Errorf("a real-time program deferred %d wakes", n)
	}
	if n := run("pingpong"); n == 0 {
		t.Errorf("a ping pong deferred no wake")
	}
	if n := run("pingpong", "XO_DEFER_WAKE=0"); n != 0 {
		t.Errorf("XO_DEFER_WAKE=0: %d wakes deferred", n)
	}
}

// TestOneThreadSpawnJoin: at XO_THREADS=1 main's spawns defer (decision
// 0077's addendum) and its join runs them: testdata/sched/spawnjoin's
// 50,000 spawns in 50 scopes wake the worker at most a few times
// (XO_RTSTATS wake), where every spawn woke it before (2026-10-16: 188
// wakes in 200,000 spawns, each a futex call of about 70 us on macOS).
// Nor does main's own runtime call wake it for a deferral it will run
// itself (mon_wake, steal): on Linux that made the worker take main's
// tasks 950 times in 200,000 spawns, two threads busy for one's work.
func TestOneThreadSpawnJoin(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	be := xotool.BackendLLVM
	if runtime.GOARCH == "arm64" {
		be = xotool.BackendArm64
	} else if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	res, err := xotool.Build([]string{filepath.Join("testdata", "sched", "spawnjoin")}, xotool.Options{Backend: be})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	cmd := exec.Command(res.Binaries[xotool.Host()])
	cmd.Env = append(os.Environ(), "XO_THREADS=1", "XO_RTSTATS=1")
	out, err := runCombined(cmd, 2*time.Minute)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !regexp.MustCompile(`spawnjoin 24975000\b`).MatchString(out) {
		t.Fatalf("output: %s", out)
	}
	m := regexp.MustCompile(`\bwake=(\d+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no wake count: %s", out)
	}
	if n, _ := strconv.Atoi(m[1]); n > 5 {
		t.Fatalf("the worker was woken %d times for 50,000 spawns its spawner joined", n)
	}
	for _, k := range []string{"mon_wake", "steal"} {
		m := regexp.MustCompile(`\b` + k + `=(\d+)`).FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("no %s count: %s", k, out)
		}
		if n, _ := strconv.Atoi(m[1]); n > 5 {
			t.Errorf("%s=%d for 50,000 spawns its spawner joined (want at most 5)", k, n)
		}
	}
}

// TestRootParkHandOff: a task the root thread runs (main's body, or a task
// main's join runs inline) does not wait for work the root would have run
// while it was parked (decision 0286): acceptbusy guards step 0 (the
// root leaves queued work to idle workers: an accept loop on the root
// waited for a computing handler), rootpark step 1 (main's body on a
// fiber: a dispatcher's send put the computing worker in the root's own
// next slot, which step 0 alone still ran). testdata/sched rootpark (a ticker's ticks to the loop task while
// a worker task computes 200 M steps; Xo Publish's repro)
// and acceptbusy (an HTTP server in main's body with two computing
// handlers, and a small request timed) print the worst delay; it must stay
// under 150 ms. Before step 0 the ticks waited about 400 ms and the small
// request about 800 ms on a Mac (2026-10-20), after it 8 ms and 2 ms. At
// XO_THREADS=1 the one worker is busy, so the root still helps and the
// delay stays until preemption (decision 0286 step 2).
//
// rootpark passes since main's body runs on a fiber (decision 0286 step
// 1): step 0 alone still ran the root's own next item, the computing
// worker a send had woken (164 ms once, 2026-10-20).
func TestRootParkHandOff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs two programs per native backend")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	worst := regexp.MustCompile(`worst (\d+) ms`)
	for _, be := range backends {
		for _, run := range []struct{ prog, threads string }{{"rootpark", ""}, {"rootpark", "4"}, {"acceptbusy", ""}, {"acceptbusy", "4"}} {
			prog := run.prog
			t.Run(be+"/"+prog+"/"+run.threads, func(t *testing.T) {
				if be == xotool.BackendLLVM {
					if _, err := xotool.Clang(); err != nil {
						t.Skip("clang not found")
					}
				}
				res, err := xotool.Build([]string{filepath.Join("testdata", "sched", prog)}, xotool.Options{Backend: be})
				if err != nil || res.Failed {
					t.Fatalf("build: %v %v", err, res)
				}
				// a second try before failing: the bound tells broken (a
				// delay of the handler's length, about 250 ms, every run)
				// from working (a few ms), and a loaded machine (the full
				// LLVM goldens alongside) once took acceptbusy to 225 ms
				ms := 0
				for try := 0; try < 2; try++ {
					cmd := exec.Command(res.Binaries[xotool.Host()])
					cmd.Env = append(os.Environ(), "XO_THREADS="+run.threads, "XO_LOCKCHECK=1")
					out, err := runCombined(cmd, 2*time.Minute)
					if err != nil {
						t.Fatalf("run: %v\n%s", err, out)
					}
					m := worst.FindStringSubmatch(out)
					if m == nil {
						t.Fatalf("output: %s", out)
					}
					ms, _ = strconv.Atoi(m[1])
					if ms < 150 {
						break
					}
					t.Logf("%s: worst %d ms on try %d", prog, ms, try+1)
				}
				if ms >= 150 {
					t.Fatalf("%s: worst delay %d ms twice (bound 150 ms; the root ran the computing work while its task was parked)", prog, ms)
				}
				t.Logf("%s: worst %d ms", prog, ms)
			})
		}
	}
}

// TestMainFiberAwaitStorm: main, on its fiber (decision 0286 step 1),
// awaits 16000 short tasks in batches of 8 (testdata/sched awaitstorm), so
// the tasks' ends signal main while its root goes idle. The root's idle
// once took the pool's lock and then main's park's while a task's end held
// the park's and took the pool's (signal, then wakep): xo's own parallel
// build hung, and awaitstorm hung in 4 of 6 runs (2026-10-20). Each run
// must end within its minute; five runs per native backend.
func TestMainFiberAwaitStorm(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program per native backend")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendLLVM {
				if _, err := xotool.Clang(); err != nil {
					t.Skip("clang not found")
				}
			}
			res, err := xotool.Build([]string{filepath.Join("testdata", "sched", "awaitstorm")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			for i := 0; i < 5; i++ {
				// the scheduler's lock order checked (pool.xo klock)
				cmd := exec.Command(res.Binaries[xotool.Host()])
				cmd.Env = append(os.Environ(), "XO_LOCKCHECK=1")
				out, err := runCombined(cmd, time.Minute)
				if err != nil {
					t.Fatalf("run %d: %v (a hang is the root's lock order)\n%s", i, err, out)
				}
				if out != "awaitstorm sum 47999\n" {
					t.Fatalf("run %d: output %q", i, out)
				}
			}
		})
	}
}

// TestRootNoSpin: the root's loop (decision 0286 step 1) steals work it
// sees queued on a busy worker. Once it saw such work, went to look without
// stealing, went idle, and saw it again, at 100% CPU (testdata/rt/rc_threads
// hung for 47 minutes under ThreadSanitizer, 2026-10-20;
// gotchas/scheduler-lock-order.md). testdata/sched stuckring makes it
// certain: with one worker (XO_THREADS=1) a task spawns a second and waits
// for it in a busy loop, so only the root can run the second; the spinning
// root hung every run of 20, the fixed one passed every run. Three runs per
// native backend, each within 30 s, with XO_LOCKCHECK.
func TestRootNoSpin(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program per native backend")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendLLVM {
				if _, err := xotool.Clang(); err != nil {
					t.Skip("clang not found")
				}
			}
			res, err := xotool.Build([]string{filepath.Join("testdata", "sched", "stuckring")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			for i := 0; i < 3; i++ {
				cmd := exec.Command(res.Binaries[xotool.Host()])
				cmd.Env = append(os.Environ(), "XO_THREADS=1", "XO_LOCKCHECK=1")
				out, err := runCombined(cmd, 30*time.Second)
				if err != nil || out != "stuckring 10\n" {
					t.Fatalf("run %d: %v (a root that does not steal never runs the waited for task)\n%s", i, err, out)
				}
			}
		})
	}
}

// TestMainFiberDeadlock: main blocked for good on a worker after its root
// went idle is still a deadlock (decision 0286 step 1). Only the root's
// idle counted main blocked, so when main parked on another worker after
// the root slept, the last worker to go idle saw no deadlock and every
// thread slept (TestCompileProperty seed 7 hung, 2026-10-21); every idle
// path now counts it (pool.xo count_main). testdata/sched mainblock hung
// 14 runs of 80 before the fix, none after: 30 runs per native backend,
// each a deadlock fault (exit 70) within 10 s.
func TestMainFiberDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program per native backend")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendLLVM {
				if _, err := xotool.Clang(); err != nil {
					t.Skip("clang not found")
				}
			}
			res, err := xotool.Build([]string{filepath.Join("testdata", "sched", "mainblock")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			for i := 0; i < 30; i++ {
				cmd := exec.Command(res.Binaries[xotool.Host()])
				cmd.Env = append(os.Environ(), "XO_LOCKCHECK=1")
				out, err := runCombined(cmd, 10*time.Second)
				var ee *exec.ExitError
				if !errors.As(err, &ee) || ee.ExitCode() != 70 || !strings.Contains(out, "deadlock") {
					t.Fatalf("run %d: %v, want the deadlock fault (exit 70)\n%s", i, err, out)
				}
			}
		})
	}
}

// TestPreempt: a fiber that runs past its slice while work waits yields at
// its loop's poll (decision 0286 step 3). testdata/sched/preempt keeps
// both threads of XO_THREADS=1 spinning on a flag only a queued task sets:
// it ends only by preemption (it hangs with XO_PREEMPT=0). Three runs per
// native backend, each within 20 s, with XO_LOCKCHECK; arm64 only, where
// the program's loops poll.
func TestPreempt(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program per native backend")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("loops poll on arm64 only")
	}
	for _, be := range []string{xotool.BackendLLVM, xotool.BackendArm64} {
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendLLVM {
				if _, err := xotool.Clang(); err != nil {
					t.Skip("clang not found")
				}
			}
			res, err := xotool.Build([]string{filepath.Join("testdata", "sched", "preempt")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			for i := 0; i < 3; i++ {
				cmd := exec.Command(res.Binaries[xotool.Host()])
				cmd.Env = append(os.Environ(), "XO_THREADS=1", "XO_LOCKCHECK=1")
				out, err := runCombined(cmd, 20*time.Second)
				if err != nil || out != "preempt 5\n" {
					t.Fatalf("run %d: %v (a spinning fiber was not preempted)\n%s", i, err, out)
				}
			}
		})
	}
}
