package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSeedPlace: scripts/lead/seed-place.sh, which seed-upload.sh runs
// after a seed upload, unpacks the seed on every offload host and checks
// each answers `xo version` (lead, 2026-11-15: seed 0.52.0 was uploaded
// but the hosts still picked 0.51.1, and a gate's seedbuild failed).
// scripts/lead/seed_place_test.sh holds the cases, against a fake
// offload.sh.
func TestSeedPlace(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "seed_place_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("seed_place_test.sh: %v\n%s", err, out)
	}
}

// TestSeedCutNeedsPushLock: scripts/seed-cut.sh refuses a cut of X.Y.0
// without XO_PUSH_LOCK and never copies its version commit onto a newer
// main (2026-11-15: the 0.52.0 cut ran without the lock, main moved, and
// the seed and tag stayed on a commit off main that the hosts' gates
// never took). scripts/lead/seed_cut_lock_test.sh holds the cases.
func TestSeedCutNeedsPushLock(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "seed_cut_lock_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("seed_cut_lock_test.sh: %v\n%s", err, out)
	}
}
