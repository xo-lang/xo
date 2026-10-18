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
