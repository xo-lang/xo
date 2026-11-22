package arm64

import (
	"os/exec"
	"strings"
	"testing"
)

// TestStdRunKeepsStderr: a failed std build's error carries its stderr
// (task #660: a gate showed only "xo dev stdcache: exit status 1").
func TestStdRunKeepsStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo std build broke >&2; exit 1")
	_, err := stdRun(cmd.Output)
	if err == nil || !strings.Contains(err.Error(), "std build broke") || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("stdRun error %v, want the exit status and the stderr", err)
	}
	if _, err := stdRun(exec.Command("true").Output); err != nil {
		t.Fatalf("stdRun of a passing command: %v", err)
	}
}
