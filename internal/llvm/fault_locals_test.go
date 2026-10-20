package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// faultLocalsProgram faults in deep (an index out of range) under step,
// whose arguments are a Str, a struct, an enum with a payload, a Float
// and a Bool, live across the faulting call.
const faultLocalsProgram = `struct Point {
  x: Int
  y: Int
}

enum Mode {
  Fast
  Slow(level: Int)
}

fn deep(k: Int) -> Int {
  let xs = [1, 2]
  xs[k]
}

fn step(label: Str, p: Point, m: Mode, ratio: Float, done: Bool, k: Int) -> Str {
  let r = deep(k)
  let lv = match m {
    Fast => 0
    Slow(level) => level
  }
  "${label} ${p.x} ${p.y} ${lv} ${ratio} ${done} ${r}"
}

fn main(os: Os) {
  var i = 0
  while i < 3 {
    os.stdio.println(step("widget-${i}", Point{x: i, y: 40 + i}, Mode.Slow(level: i + 5), 0.25 * Float(i), i > 1, i))
    i += 1
  }
}
`

// TestFaultLocals runs a faulting program with XO_TRACEBACK=locals
// (decision 0268) on each native backend and requires the frames'
// variables in the report, read from the program's own DWARF at the
// fault (before the unwinding releases them); without it, the report is
// the plain traceback.
func TestFaultLocals(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program on each native backend")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the DWARF is read on Linux and macOS")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(faultLocalsProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/faultlocals\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		// arm64: the arguments of the frame that called the faulting one
		xotool.BackendArm64: {`label = "widget-2"`, `p = Point { x: 2, y: 42 }`, `m = Mode.Slow(level: 7)`, `ratio = 0.5`, `done = true`, `i = 2`},
		// LLVM inlines deep and step into main: their variables in scope
		xotool.BackendLLVM: {`i = 2`, `k = 2`, `xs = [1, 2]`},
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res.Output)
			}
			bin := res.Binaries[xotool.Host()]
			run := func(tb string) string {
				cmd := exec.Command(bin)
				cmd.Env = append(os.Environ(), "XO_TRACEBACK="+tb)
				testtmp.Lifeline(t, cmd, 2*time.Minute)
				out, _ := cmd.CombinedOutput()
				return string(out)
			}
			got := run("locals")
			if !strings.Contains(got, "fault: index 2 out of range") {
				t.Fatalf("no fault report:\n%s", got)
			}
			for _, w := range want[be] {
				if !strings.Contains(got, "      "+w+"\n") {
					t.Errorf("XO_TRACEBACK=locals: no line %q in\n%s", w, got)
				}
			}
			if plain := run("1"); strings.Contains(plain, " = ") || !strings.Contains(plain, "traceback:") {
				t.Errorf("XO_TRACEBACK=1: want the frames without variables, got\n%s", plain)
			}
		})
	}
}

// faultFrameProgram faults (a division by zero) in total itself, a
// function that calls (depth, recursive, so not inlined; total calls
// itself too, so it stays a frame of its own): its variables
// live across that call in callee saved registers or spill slots, which
// the faulting frame keeps at the fault stub's call.
const faultFrameProgram = `fn depth(n: Int) -> Int {
  if n <= 0 {
    return 0
  }
  1 + depth(n - 1)
}

fn total(xs: List[Int], k: Int) -> Int {
  if k > 100 {
    // never taken: a call of itself keeps total out of line
    return total(xs, k - 1)
  }
  var s = 0
  for x in xs {
    s += depth(x)
    s += x / k
  }
  s
}

fn main(os: Os) {
  let xs = [1, 2, 3]
  os.stdio.println("${total(xs, 1)}")
  os.stdio.println("${total(xs, xs.len() - 3)}")
}
`

// TestFaultLocalsFaultingFrame: the faulting frame's own variables
// (2026-10-20: every one printed <unavailable>, their ranges ending before
// the out of line fault stub the traceback returns into; dwvars.xo
// DvStub). arm64; the amd64 backend writes no DWARF variables yet.
func TestFaultLocalsFaultingFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend's DWARF, on Linux and macOS")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(faultFrameProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/faultframe\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res.Output)
	}
	cmd := exec.Command(res.Binaries[xotool.Host()])
	cmd.Env = append(os.Environ(), "XO_TRACEBACK=locals")
	testtmp.Lifeline(t, cmd, 2*time.Minute)
	out, _ := cmd.CombinedOutput()
	got := string(out)
	if !strings.Contains(got, "fault: division by zero") {
		t.Fatalf("no fault report:\n%s", got)
	}
	// total's frame, the first after "traceback:"
	i := strings.Index(got, "  total (")
	if i < 0 {
		t.Fatalf("no total frame:\n%s", got)
	}
	// up to the next frame's line (two spaces, then its name)
	frame := got[i:]
	lines := strings.Split(frame, "\n")
	end := len(lines)
	for n := 1; n < len(lines); n++ {
		if strings.HasPrefix(lines[n], "  ") && !strings.HasPrefix(lines[n], "   ") {
			end = n
			break
		}
	}
	frame = strings.Join(lines[:end], "\n") + "\n"
	// k and x live across depth's call (callee saved registers); xs is
	// read through its data and length only, and s's latest sum is in a
	// caller saved register the fault path does not keep
	for _, w := range []string{"k = 0", "x = 1"} {
		if !strings.Contains(frame, "      "+w+"\n") {
			t.Errorf("XO_TRACEBACK=locals: total's frame lacks %q:\n%s", w, got)
		}
	}
}

// faultInlinedProgram faults in pick, which the inliner puts into main:
// pick's own variables are described under its DW_TAG_inlined_subroutine
// (2026-10-20: dwvars.xo skipped every inlined variable, so the frame
// had none of them).
const faultInlinedProgram = `fn pick(xs: List[Int], k: Int) -> Int {
  xs[k]
}

fn main(os: Os) {
  let xs = [1, 2]
  var i = 0
  while i < 3 {
    os.stdio.println("${pick(xs, i)}")
    i += 1
  }
}
`

// TestFaultLocalsInlined: an inlined callee's variables in the report
// (arm64, as the LLVM backend shows them: in scope where the call was
// inlined).
func TestFaultLocalsInlined(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	if runtime.GOARCH != "arm64" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("the arm64 backend's DWARF, on Linux and macOS")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(faultInlinedProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/faultinl\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res.Output)
	}
	cmd := exec.Command(res.Binaries[xotool.Host()])
	cmd.Env = append(os.Environ(), "XO_TRACEBACK=locals")
	testtmp.Lifeline(t, cmd, 2*time.Minute)
	out, _ := cmd.CombinedOutput()
	got := string(out)
	if !strings.Contains(got, "fault: index 2 out of range") {
		t.Fatalf("no fault report:\n%s", got)
	}
	if !strings.Contains(got, "pick (main.xo:2) [inlined]") {
		t.Skipf("pick was not inlined; nothing to check:\n%s", got)
	}
	if !strings.Contains(got, "      k = 2\n") {
		t.Errorf("XO_TRACEBACK=locals: no line \"k = 2\" (pick's parameter) in\n%s", got)
	}
}
