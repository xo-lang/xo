package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestHeavydRestartRetry: scripts/lead/heavyd-restart.sh boots heavyd out
// and in, retrying a failed bootstrap once (lead, 2026-11-21: the first
// bootstrap after a bootout ended rc 5, Input/output error, and a retry
// 5 s later worked). scripts/lead/heavyd_restart_test.sh holds the cases.
func TestHeavydRestartRetry(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "heavyd_restart_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("heavyd_restart_test.sh: %v\n%s", err, out)
	}
}
