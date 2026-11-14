package arm64

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// arm64GoldenSelected reports whether golden program name runs in the arm64
// tests that build every golden of internal/llvm (TestGoldenRun,
// TestGoldenRunSpill, TestGoldenRunHeapGuard, TestGoldenPassesOff,
// TestDebugInfoSameCode): every one when XO_ARM64_GOLDENS is unset; none
// when it is "none"; else the comma separated names. scripts/gate.sh sets
// it to the llvm step's selection (the goldens whose LLVM IR or directory
// the change alters) when the change leaves the native backend alone
// (lead, 2026-11-14, owner's test-time plan: TestGoldenRun alone was
// 3,523 s of subtests in a host 2 gate; bench/results/gate-time-2026-11-14.md).
func arm64GoldenSelected(name string) bool {
	v, ok := os.LookupEnv("XO_ARM64_GOLDENS")
	if !ok {
		return true
	}
	if v == "none" {
		return false
	}
	for _, n := range strings.Split(v, ",") {
		if strings.TrimSpace(n) == name {
			return true
		}
	}
	return false
}

// skipUnselectedArm64 skips a golden's subtest that XO_ARM64_GOLDENS leaves
// out.
func skipUnselectedArm64(t *testing.T, dir string) {
	t.Helper()
	if !arm64GoldenSelected(filepath.Base(dir)) {
		t.Skip("not selected (XO_ARM64_GOLDENS: its LLVM IR and directory are unchanged, and the change leaves the native backend alone)")
	}
}
