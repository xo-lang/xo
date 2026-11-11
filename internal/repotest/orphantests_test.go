package repotest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoOrphanStdTests: every std/native/M_test.xo has its std/native/M.xo.
// A merge (decision 0588) that moves a module's tests into std/M_test.xo
// and leaves the native test file makes `xo test --std-module=M`
// ambiguous (native/M or std/M) and names a native module that is gone
// (2026-11-11: std/netip's merge, 818fa73bb, failed internal/arm64
// TestFrontCacheAcrossPrograms on main).
func TestNoOrphanStdTests(t *testing.T) {
	tests, err := filepath.Glob(filepath.Join(root, "std", "native", "*_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range tests {
		m := strings.TrimSuffix(filepath.Base(f), "_test.xo")
		if _, err := os.Stat(filepath.Join(root, "std", "native", m+".xo")); err != nil {
			t.Errorf("std/native/%s_test.xo has no std/native/%s.xo: a merge left it (move its tests into std/%s_test.xo and delete it)", m, m, m)
		}
	}
}
