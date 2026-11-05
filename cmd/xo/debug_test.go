package main

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	res, err := xotool.Build([]string{dir}, buildOpts(t, xotool.Options{Backend: "arm64", Out: filepath.Join(t.TempDir(), "prog")}))
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
func TestDebugBatch(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("xo debug's process control is macOS arm64 so far (decision 0253 stage 2)")
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
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("xo debug's process control is macOS arm64 so far (decision 0253 stage 2)")
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
