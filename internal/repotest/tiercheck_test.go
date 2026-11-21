package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestTierCheck runs scripts/lead/tier-check_test.sh: tier-check.py, the
// release readiness check of decision 0730 (task #567), on small tier logs
// in tier.sh's formats (every step PASS, FAIL or NO-RESULT; a no-result
// step counts as failing).
func TestTierCheck(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	s := filepath.Join("..", "..", "scripts", "lead", "tier-check_test.sh")
	out, err := exec.Command("bash", s).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", s, err, out)
	}
}
