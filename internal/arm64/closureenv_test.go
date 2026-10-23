package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestClosureEnvInFrame requires a closure whose captures hold nothing
// counted, and whose value is only called, to keep its environment in
// the frame (closures.xo stack_closures): the golden's 20,000
// Map.update calls with a closure capturing the loop index allocated an
// environment each (21,073 allocations, 2026-10-23); the golden's
// escaping closures keep theirs on the heap.
func TestClosureEnvInFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	dir := filepath.Join("..", "llvm", "testdata", "golden", "closure_env_frame")
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
	cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	m := leakRE.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("no leak check line:\n%s", out)
	}
	total, _ := strconv.Atoi(m[2])
	if m[1] != "0" || total > 400 {
		t.Errorf("%s live of %d allocations, want 0 of at most 400 (an environment allocated per call?)", m[1], total)
	}
}
