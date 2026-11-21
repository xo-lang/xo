package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPushqFastSkip: pushq.sh's fast checks skip a slow internal/repotest
// test only when a push changes none of its inputs, and the input table
// (pq_fast_slow) is closed: every script a listed test runs, through
// $here or $(dirname "$0") references followed to the end, is under one
// of its prefixes (task #522: a narrowed list nothing checked would go
// stale as scripts start calling new ones).
// scripts/lead/pushq_fast_skip_test.sh holds the cases; about 1 s.
func TestPushqFastSkip(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "pushq_fast_skip_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("pushq_fast_skip_test.sh: %v\n%s", err, out)
	}
}
