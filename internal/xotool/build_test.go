package xotool

import (
	"path/filepath"
	"sync"
	"testing"
)

// resetPath makes the next Path resolve the xo again.
func resetPath() {
	pathOnce = sync.Once{}
	pathBin, pathErr = "", nil
}

// TestBuildResultNeverNil: a Build that cannot start xo still returns a
// Result, so callers that print res.Output beside the error do not panic
// (a stale XO_CLI panicked TestDebugDAP on Windows, 2026-11-05).
func TestBuildResultNeverNil(t *testing.T) {
	// Path resolves once per process: forget the missing xo afterwards,
	// or every later test of the package found no xo (TestBuildTempCleaned
	// skipped, TestTierSepcompWarm failed)
	resetPath()
	t.Cleanup(resetPath)
	t.Setenv("XO_CLI", filepath.Join(t.TempDir(), "no-such-xo"))
	res, err := Build([]string{t.TempDir()}, Options{})
	if err == nil {
		t.Fatal("Build with a missing xo: no error")
	}
	if res == nil {
		t.Fatalf("Build with a missing xo: nil Result (%v)", err)
	}
}
