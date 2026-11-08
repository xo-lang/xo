package repotest

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGateStepsShared: scripts/gate.sh and scripts/lead/pushq.sh decide
// pushq's proving checks (codegen, wasm, cross, selfbuild) through one
// set of functions, scripts/lead/gatelib.sh's gate_required_steps,
// gate_plan and gate_missing, and a check a gate names runs beside the
// ones the paths require (lead, 2026-11-08: a gate named `cross codegen`,
// ran neither, and pushq then refused the push for cross).
// scripts/lead/gatesteps_test.sh holds the cases.
func TestGateStepsShared(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "gatesteps_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("gatesteps_test.sh: %v\n%s", err, out)
	}
}
