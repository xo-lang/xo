package benchharness

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/build"
)

// TestHarness runs bench/harness/test_harness.py. It needs python3 and the
// go command on PATH, and never calls an external endpoint (the adapter
// test uses a fake server on 127.0.0.1). About 7 seconds.
//
// The xo commands it runs reuse built binaries (the binary cache) and
// their -race test binaries do not sleep 1 s at exit (GORACE
// atexit_sleep_ms=0, gotchas/race-binaries-sleep-at-exit.md); together
// 24 s became 7 s on 2026-10-09.
func TestHarness(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "bench", "harness", "test_harness.py"))
	if err != nil {
		t.Fatal(err)
	}
	build.EnableTestBinCache()
	cmd := exec.Command(py, "-I", script)
	cmd.Env = os.Environ()
	if _, ok := os.LookupEnv("GORACE"); !ok {
		cmd.Env = append(cmd.Env, "GORACE=atexit_sleep_ms=0")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("harness self test failed: %v\n%s", err, out)
	}
}
