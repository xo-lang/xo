package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunCacheMissReason: after a run cache miss, --phase-times says why
// ("cache run miss: ...", the first recorded input that no longer holds)
// and prints the run key's parts, so two builds that should share an
// entry can be compared (2026-10-30: a Linux miss on every build was a
// /proc/meminfo read, found by bisecting).
func TestRunCacheMissReason(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	backends := nativeBackends(t)
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	dir := filepath.Join(tmp, "prog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.xo")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(main, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/prog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(what string) string {
		t.Helper()
		var out, errb bytes.Buffer
		if code := run(t, []string{"build", "--backend=" + backends[0], "--phase-times", "-o", filepath.Join(tmp, "prog.bin"), dir}, &out, &errb); code != 0 {
			t.Fatalf("%s: exit %d\n%s%s", what, code, out.String(), errb.String())
		}
		return out.String() + errb.String()
	}
	write("fn main(os: Os) {\n  os.stdio.println(\"one\")\n}\n")
	if log := build("first build"); !strings.Contains(log, "cache run miss: no entry for key ") || !strings.Contains(log, "cache run key ") {
		t.Errorf("first build: want the miss's reason and the key's parts:\n%s", log)
	}
	if log := build("unchanged"); strings.Contains(log, "cache run miss") {
		t.Errorf("unchanged build: want a run cache hit:\n%s", log)
	}
	write("fn main(os: Os) {\n  os.stdio.println(\"two\")\n}\n")
	if log := build("edited"); !strings.Contains(log, "cache run miss: file changed: ") || !strings.Contains(log, "main.xo") {
		t.Errorf("edited build: want the miss named at main.xo:\n%s", log)
	}
}
