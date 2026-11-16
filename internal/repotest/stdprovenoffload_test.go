package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestStdprovenOffloadNoBundle: stdproven-offload.sh sends no bundle when
// the host already has HEAD and origin/main (its sent ref, as right after
// a push), and stdproven-remote.sh runs on the commits it has (azure-1,
// 2026-11-16: git refused the empty bundle and the evidence run ended rc
// 2). scripts/lead/stdproven_offload_test.sh holds the cases; nothing runs
// on a host.
func TestStdprovenOffloadNoBundle(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "stdproven_offload_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("stdproven_offload_test.sh: %v\n%s", err, out)
	}
}
