package xotool

import (
	"path/filepath"
	"testing"
)

// TestBuildResultNeverNil: a Build that cannot start xo still returns a
// Result, so callers that print res.Output beside the error do not panic
// (a stale XO_CLI panicked TestDebugDAP on Windows, 2026-11-05).
func TestBuildResultNeverNil(t *testing.T) {
	t.Setenv("XO_CLI", filepath.Join(t.TempDir(), "no-such-xo"))
	res, err := Build([]string{t.TempDir()}, Options{})
	if err == nil {
		t.Fatal("Build with a missing xo: no error")
	}
	if res == nil {
		t.Fatalf("Build with a missing xo: nil Result (%v)", err)
	}
}
