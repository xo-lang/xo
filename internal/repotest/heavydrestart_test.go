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

// TestTierMemWait: scripts/lead/tier-memwait.sh, tier.sh's free-memory
// wait before a Mac step, is skipped while xw's admission row has fit on
// and need_gb set (decision 0732 step 2, #600); cases in
// scripts/lead/tier_memwait_test.sh.
func TestTierMemWait(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "tier_memwait_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("tier_memwait_test.sh: %v\n%s", err, out)
	}
}
