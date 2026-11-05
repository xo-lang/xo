package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// debugProg is xo debug's golden program: sum's loop inlined into main,
// a struct argument, four turns of the loop.
const debugProg = `struct Point {
  x: Int
  y: Int
}

fn sum(p: Point, n: Int) -> Int {
  var total = 0
  for i in 0..n {
    total += p.x * i + p.y
  }
  total
}

fn main(os: Os) {
  let n = os.env.args().len() + 4
  let p = Point{x: n * 2, y: 7}
  let t = sum(p, n)
  os.stdio.println("sum ${t}")
}
`

// debugFact is stage 3's: a call that is not inlined (recursion).
const debugFact = `fn fact(n: Int) -> Int {
  if n <= 1 {
    return 1
  }
  let r = fact(n - 1)
  n * r
}

fn main(os: Os) {
  let n = os.env.args().len() + 3
  let f = fact(n)
  os.stdio.println("fact ${f}")
}
`

// debugBatch builds src as main.xo with the arm64 backend and runs it
// under xo debug --batch with cmds: its standard output, skipping the
// test when this xo cannot control a process.
func debugBatch(t *testing.T, src string, cmds []string) string {
	t.Helper()
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": src})
	res, err := xotool.Build([]string{dir}, buildOpts(t, xotool.Options{Backend: debugBackend(), Out: filepath.Join(t.TempDir(), "prog")}))
	if err != nil || res.Failed {
		t.Fatalf("build: %v %s", err, res.Output)
	}
	prog := res.Binaries[xotool.Host()]
	var out, errb bytes.Buffer
	args := []string{"debug", "--batch"}
	for _, c := range cmds {
		args = append(args, "-c", c)
	}
	code := runXo(t, append(args, prog), &out, &errb)
	got := out.String()
	if strings.Contains(got+errb.String(), "task_for_pid") {
		// an xo built by a seed older than --entitlement (xotool
		// debuggerArgs) for a program without get-task-allow
		t.Skipf("this xo cannot control the program (entitlements, Developer Mode):\n%s%s", got, errb.String())
	}
	if code != 0 {
		t.Fatalf("xo debug: exit %d\n%s%s", code, got, errb.String())
	}
	return got
}

// wantAll reports each of wants missing from got.
func wantAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
}

// TestDebugBatch runs a native program under `xo debug --batch` (decision
// 0253 stage 2, macOS arm64): a breakpoint in a loop, its locals, a field,
// the backtrace through the inlined frame, each of the four stops with the
// loop's values, then the program's exit.
// debugLaunches: whether xo debug starts programs on this machine
// (decision 0253: macOS arm64 at stage 2, Linux arm64 and amd64 at
// stage 6).
func debugLaunches() bool {
	if runtime.GOOS == "linux" {
		return runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64"
	}
	return runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
}

// debugBackend: the native backend of this machine's programs (xo
// build --backend: arm64, or amd64 on x86_64 Linux).
func debugBackend() string {
	if runtime.GOARCH == "amd64" {
		return "amd64"
	}
	return xotool.BackendArm64
}

func TestDebugBatch(t *testing.T) {
	if !debugLaunches() {
		t.Skip("xo debug's process control is macOS arm64 and Linux so far (decision 0253 stages 2 and 6)")
	}
	if testing.Short() {
		t.Skip("builds and debugs a native program")
	}
	got := debugBatch(t, debugProg, []string{"b main.xo:9", "run", "locals", "p p.x", "bt", "c", "p total", "p i", "c", "c", "c", "threads"})
	for _, want := range []string{
		"breakpoint 1: 0x",
		"(xo) run\nstopped at breakpoint 1: 0x",
		" sum [inlined] at main.xo:9\n(xo) locals\np = Point{x: 8, y: 7}\nn = 4\ntotal = 0\ni = 0\n",
		"(xo) p p.x\np.x = 8\n",
		" main at main.xo:17\n",
		"(xo) p total\ntotal = 7\n(xo) p i\ni = 1\n",
		"(xo) c\nthe program exited with status 0\n",
		"(xo) threads\nno threads (the program is not stopped)\n",
		"sum 76\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "stopped at breakpoint 1:"); n != 4 {
		t.Errorf("%d stops at the breakpoint, want 4 (one a turn of the loop):\n%s", n, got)
	}
}

// TestDebugStep is stage 3 (decision 0253): next over an inlined call
// and by lines (and stopped by a breakpoint in the call it runs over),
// step into an inlined call and a real one, finish out of
// an inlined call (its caller's frame selected, the result in a local)
// and out of each recursive call, the stack pointer telling the frames
// apart.
func TestDebugStep(t *testing.T) {
	if !debugLaunches() {
		t.Skip("xo debug's process control is macOS arm64 and Linux so far (decision 0253 stages 2 and 6)")
	}
	if testing.Short() {
		t.Skip("builds and debugs native programs")
	}
	got := debugBatch(t, debugProg, []string{"b main.xo:15", "run", "n", "n", "d 1", "b main.xo:9", "c", "s", "s", "p total", "finish", "locals", "c"})
	wantAll(t, got,
		"(xo) n\n0x", " main at main.xo:16\n(xo) n\n0x", " main at main.xo:17\n(xo) d 1\n",
		"(xo) s\n0x", " sum [inlined] at main.xo:8\n(xo) s\n0x", " sum [inlined] at main.xo:9\n(xo) p total\ntotal = 7\n",
		"(xo) finish\n0x", " main at main.xo:18\n(xo) locals\n", "t = 76\n",
		"the program exited with status 0\n", "sum 76\n")
	got = debugBatch(t, debugFact, []string{"b main.xo:11", "run", "s", "p n", "n", "d 1", "b fact", "n", "p n", "c", "p n", "d 2", "finish", "p n", "locals", "finish", "p n", "finish", "c"})
	wantAll(t, got,
		"(xo) s\n0x", " fact at main.xo:2\n(xo) p n\nn = 3\n(xo) n\n0x", " fact at main.xo:5\n",
		// a breakpoint in the call next runs over stops it
		"(xo) n\nstopped at breakpoint 2: 0x", " fact at main.xo:2\n(xo) p n\nn = 2\n",
		"(xo) p n\nn = 1\n(xo) d 2\n",
		"(xo) finish\n0x", " fact at main.xo:6\n(xo) p n\nn = 2\n(xo) locals\nn = 2\nr = 1\n",
		" fact at main.xo:6\n(xo) p n\nn = 3\n(xo) finish\n0x", " main at main.xo:11\n",
		"the program exited with status 0\n", "fact 6\n")
}

// debugTasks has, at the breakpoint in its third task, one task asleep
// inside a Mutex it holds and one blocked on that Mutex (the program of
// TestDump).
const debugTasks = `fn main(os: Os) {
  let m = Mutex[Int].new(0)
  let held = Chan[Int].new(cap: 1)
  scope s {
    s.spawn(fn() {
      m.with(fn(var v) {
        held.send(1)
        os.clock.sleep(1h)
        v += 1
      })
    })
    s.spawn(fn() {
      let _ = held.recv()
      m.with(fn(v) { v })
    })
    s.spawn(fn() {
      os.clock.sleep(200ms)
      os.stdio.println("mark")
    })
  }
}
`

// TestDebugTasks checks decision 0253 stage 4 on a live program: tasks
// lists the task tree from the runtime's records as xo dump does, task N
// selects a blocked task's frame in the program's code (its fiber's
// stack), and locals and print read it there.
func TestDebugTasks(t *testing.T) {
	if !debugLaunches() {
		t.Skip("xo debug's process control is macOS arm64 and Linux so far (decision 0253 stages 2 and 6)")
	}
	if testing.Short() {
		t.Skip("builds and debugs a native program")
	}
	got := debugBatch(t, debugTasks, []string{"break main.xo:18", "run", "tasks", "task 2", "print v", "task 3 bt", "task 4"})
	wantAll(t, got,
		"(xo) tasks\nmain (task 1): in scope\n  scope: waiting for its tasks\n    task 2 (spawned at main.xo:5): blocked in sleep at main.xo:8\n    task 3 (spawned at main.xo:12): blocked in Mutex.with at main.xo:14\n    task 4 (spawned at main.xo:16): running\n",
		"(xo) task 2\ntask 2 (spawned at main.xo:5): blocked in sleep at main.xo:8\n* #",
		"main.fn1.fn4 at main.xo:8\n(xo) print v\nv = 0\n",
		"mutex.lock_body at mutex.xo:",
		"main.fn2 at main.xo:14\n",
		// the running task: on the stopped thread, at the breakpoint
		"(xo) task 4\ntask 4 (spawned at main.xo:16): running\n* #0 0x",
		"main.fn3 at main.xo:18\n",
	)
}

// TestDebugAttach checks `xo debug -p PID PROG` (decision 0253 stage 4):
// attached to debugTasks running on its own (ASLR on: the slide is
// found from its threads), tasks and task read its records, and after
// the session the program runs on (detached, not killed).
func TestDebugAttach(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("xo debug attaches on macOS arm64 so far (decision 0253 stage 4; Linux's needs /proc/PID/task, stage 6)")
	}
	if testing.Short() {
		t.Skip("builds and debugs a native program")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": debugTasks})
	res, err := xotool.Build([]string{dir}, buildOpts(t, xotool.Options{Backend: debugBackend(), Out: filepath.Join(t.TempDir(), "prog")}))
	if err != nil || res.Failed {
		t.Fatalf("build: %v %s", err, res.Output)
	}
	prog := res.Binaries[xotool.Host()]
	p := exec.Command(prog)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		p.Process.Kill()
		p.Wait()
	}()
	// attached until both tasks are blocked (its output is buffered in a
	// pipe, so it cannot say when); each attach leaves it running
	var got string
	for try := 0; ; try++ {
		var stdout, stderr bytes.Buffer
		code := runXo(t, []string{"debug", "-p", strconv.Itoa(p.Process.Pid), "--batch", "-c", "tasks", "-c", "task 2", "-c", "print v", prog}, &stdout, &stderr)
		got = stdout.String()
		if strings.Contains(got+stderr.String(), "task_for_pid") {
			t.Skipf("this xo cannot control the program (entitlements, Developer Mode):\n%s%s", got, stderr.String())
		}
		if code == 0 && strings.Contains(got, "blocked in Mutex.with") {
			break
		}
		if try == 20 {
			t.Fatalf("xo debug -p: exit %d\n%s%s", code, got, stderr.String())
		}
		time.Sleep(250 * time.Millisecond)
	}
	wantAll(t, got,
		"(xo) attach "+strconv.Itoa(p.Process.Pid)+"\nattached to process "+strconv.Itoa(p.Process.Pid)+" (",
		"threads, every one stopped; slide 0x",
		"    task 2 (spawned at main.xo:5): blocked in sleep at main.xo:8\n    task 3 (spawned at main.xo:12): blocked in Mutex.with at main.xo:14\n",
		"main.fn1.fn4 at main.xo:8\n(xo) print v\nv = 0\n",
	)
	if err := p.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("the program did not survive the detach: %v", err)
	}
}
