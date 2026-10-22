package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestHybridDeprecated: `xo build --hybrid` still builds (a plain Go
// backend build) and warns that the flag is deprecated (owner,
// 2026-10-22).
func TestHybridDeprecated(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds with the Go backend")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	})
	code, out := xoOut(t, "build", "--hybrid", "-o", filepath.Join(t.TempDir(), "app"), filepath.Join(dir, "app"))
	if code != 0 || !strings.Contains(out, "--hybrid is deprecated") {
		t.Errorf("exit %d\n%s", code, out)
	}
}
