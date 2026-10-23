package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestTracebackLocalsTypes: XO_TRACEBACK=locals (decision 0268) on the
// host's native backend (arm64, or amd64 on an x86 host) shows a frame's
// variables of several types as the spec describes: numbers, Bool, Str
// text, a struct, an enum with a payload, an Option, a List's first
// elements of any whole-word element type (List[Str] showed only its
// length), Os by its type, and no line for a local not set yet at the
// fault (`let r = deep(k)` showed <unavailable>). (Intel field report,
// round 2, on 0.26.0; darwin/amd64 printed the same on the Intel laptop,
// 2026-10-23.)
func TestTracebackLocalsTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	be := xotool.BackendArm64
	switch {
	case runtime.GOARCH == "amd64":
		be = "amd64"
	case !native():
		t.Skip("no native backend for this host")
	}
	bin := filepath.Join(t.TempDir(), "locals")
	r, err := xotool.Build([]string{filepath.Join("testdata", "traceback_locals")}, xotool.Options{Backend: be, Out: bin})
	if err != nil || r.Failed {
		t.Fatalf("build: %v\n%s", err, r.Output)
	}
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Env = append(os.Environ(), "XO_TRACEBACK=locals")
	var stderr strBuf
	cmd.Stderr = &stderr
	_ = cmd.Run()
	want := `fault: index 2 out of range for List of length 2 at main.xo:17:3
traceback:
  deep (main.xo:17)
      k = 2
      xs = [1, 2]
  step (main.xo:21) [inlined]
      label = "widget-2"
      p = Point { x: 2, y: 42 }
      m = Mode.Slow(level: 7)
      ratio = 0.5
      done = true
      k = 2
      names = ["a-2", "b"]
      nums = [2, 7, 9]
      opt = Int?.Some(v: 20)
      level = 7
  main (main.xo:32)
      os = Os
      i = 2
`
	if got := stderr.String(); got != want {
		t.Errorf("XO_TRACEBACK=locals on %s:\n%s\nwant:\n%s", be, got, want)
	}
}

type strBuf struct{ b []byte }

func (s *strBuf) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }
func (s *strBuf) String() string              { return string(s.b) }
