package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestOffloadGateLease: offload-gate.sh sizes its lease from pushq.sh
// --needs's steps when none are named, and pushq.sh --gated refuses a
// change whose last gate ended other than 0 (aws-43, 2026-11-14: a gate
// that named no steps was stopped at its 15 minute lease, rc 124, and
// pushed on a misread). scripts/lead/offload_gate_test.sh holds the cases;
// nothing runs on a host.
func TestOffloadGateLease(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "offload_gate_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("offload_gate_test.sh: %v\n%s", err, out)
	}
}
