package repotest

import (
	"os"
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
	// pushq runs this test inside heavyd's merge lane, whose HEAVY_LANE
	// skipped the lane and host 1 checks as for a job inside a lease (the
	// lead tools batch's push, 2026-11-17); a caller's routing must not
	// reach the cases
	env := append(os.Environ(), "HEAVY_LANE=merge", "HEAVY_LANE_SLOT=1",
		"HEAVY_LANE_ENV=offload-arm64b.env", "OFFLOAD_ENV=offload-arm64c.env",
		"AB_HOST_AGENT=caller")
	for _, e := range [][]string{os.Environ(), env} {
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "lead", "offload_gate_test.sh"))
		cmd.Env = e
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("offload_gate_test.sh: %v\n%s", err, out)
		}
	}
}

// TestOffloadAmd64Exclusive runs scripts/lead/offload_amd64_excl_test.sh:
// on the amd64 host only a timing (OFF_TIMING=1) takes every slot; any
// other OFF_EXCLUSIVE=1 job is a 2-slot memory lease (task #370), also
// when the caller runs inside a heavyd lane.
func TestOffloadAmd64Exclusive(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	// pushq runs it inside heavyd's merge lane: with HEAVY_LANE set
	// offload.sh skipped the lease and the cases failed on main
	// (2026-11-18, 2a630e23a); a caller's lane must not reach them
	env := append(os.Environ(), "HEAVY_LANE=merge", "HEAVY_LANE_SLOT=1",
		"HEAVY_LANE_ENV=offload-arm64b.env", "OFF_NO_LANE=1")
	for _, e := range [][]string{os.Environ(), env} {
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "lead", "offload_amd64_excl_test.sh"))
		cmd.Env = e
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("offload_amd64_excl_test.sh: %v\n%s", err, out)
		}
	}
}
