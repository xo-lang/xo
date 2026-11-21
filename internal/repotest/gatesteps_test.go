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

// TestGateLLVMTouchAll: scripts/gate.sh's llvm_touch_all, the check that
// makes the llvm step run every golden, answers false under
// XO_GATE_BIN_SELECT=force, as bin_select's own check does (decision 0657
// stage 2). scripts/lead/gate_llvm_touch_test.sh holds the cases.
func TestGateLLVMTouchAll(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "gate_llvm_touch_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("gate_llvm_touch_test.sh: %v\n%s", err, out)
	}
}

// TestGateIRShare: scripts/gate.sh's ir_share shares the base manifest a
// gate built for the next gate on the same base and host, and ir_source
// names who made a shared one (decision #87, task #275).
// scripts/lead/gate_ir_share_test.sh holds the cases.
func TestGateIRShare(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "gate_ir_share_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("gate_ir_share_test.sh: %v\n%s", err, out)
	}
}

// TestGateStepCanaries: every gate step of scripts/lead/gate-needs.txt has
// a planted bad input that its canary (scripts/lead/gatesteps-canary.sh,
// scheduled on a host) gives it, and each plant applies to this checkout
// (task #360: a step that skipped its tests still printed its pass line,
// b62292196). scripts/lead/gatesteps_canary_test.sh holds the cases.
func TestGateStepCanaries(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "gatesteps_canary_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("gatesteps_canary_test.sh: %v\n%s", err, out)
	}
}

// TestPushqStrayCheck: pushq refuses a squash by soft reset onto a moved
// origin/main, which sets back the paths main changed since the branch
// began (task #360; f0f5440f3 reverted 17 files, 55ac2cd33 restored them).
// scripts/lead/straycheck_test.sh holds the cases.
func TestPushqStrayCheck(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "straycheck_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("straycheck_test.sh: %v\n%s", err, out)
	}
}

// TestCgcheck runs scripts/lead/cgcheck_test.sh: the scheduled callgrind
// check's comparison (+3% regresses) and its base (the last green
// ancestor run), task #364.
func TestCgcheck(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "lead", "cgcheck_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("cgcheck_test.sh: %v\n%s", err, out)
	}
}

// TestGateRtStringGoldens: a std/rt/*.go change runs on the Go backend the
// internal/llvm goldens whose want file holds a string it adds or removes
// (scripts/gate.sh's rt_string_goldens, scripts/golden-rt-strings.py):
// std/rt is no xo build input, so the LLVM selection named none of #540's
// four BytesBuilder.put goldens and the every-16th sample missed them
// (task #552). scripts/golden-rt-strings_test.sh holds the cases.
func TestGateRtStringGoldens(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "golden-rt-strings_test.sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("golden-rt-strings_test.sh: %v\n%s", err, out)
	}
}
