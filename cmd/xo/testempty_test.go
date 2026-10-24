package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestXoTestNoTests: `xo test` on a package with no tests says so on
// standard error and exits 0, and with --fail-empty exits 1 (2026-10-24:
// a gate's `xo test --std-module=time`, a module with no tests, printed
// "ok: 0 passed" and read as a pass).
func TestXoTestNoTests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) uses (stdio) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/notests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run(t, []string{"test", dir}, &out, &errb); code != 0 || !strings.Contains(errb.String(), "xo test: no tests in") {
		t.Fatalf("xo test: want exit 0 and a no tests line, got %d:\n%s%s", code, out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run(t, []string{"test", "--fail-empty", dir}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "(--fail-empty)") {
		t.Fatalf("xo test --fail-empty: want exit 1, got %d:\n%s%s", code, out.String(), errb.String())
	}
}
