package benchharness

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestHarness runs bench/harness/test_harness.py. It needs python3 and the
// go command on PATH, and never calls an external endpoint (the adapter
// test uses a fake server on 127.0.0.1). About 7 seconds.
func TestHarness(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "bench", "harness", "test_harness.py"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(py, "-I", script).CombinedOutput()
	if err != nil {
		t.Fatalf("harness self test failed: %v\n%s", err, out)
	}
}
