package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestHolesNative: `xo run --holes` builds natively (owner, 2026-10-22):
// the program runs up to the hole, which faults with the Go backend's
// message; without --holes the build is rejected.
func TestHolesNative(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn later(n: Int) -> Int { ??? }\n\nfn main(os: Os) {\n  os.stdio.println(\"before\")\n  os.stdio.println(\"${later(1)}\")\n}\n",
	})
	app := filepath.Join(dir, "app")
	for _, be := range nativeBackends(t) {
		code, out := xoOut(t, "run", "--holes", "--backend="+be, app)
		if code != 70 || !strings.Contains(out, "before\n") || !strings.Contains(out, "evaluated a hole `???` at main.xo:1:") {
			t.Errorf("%s: exit %d\n%s", be, code, out)
		}
	}
	code, out := xoOut(t, "run", "--holes", app)
	if strings.Contains(out, "does not support --holes") || !strings.Contains(out, "evaluated a hole") {
		t.Errorf("default backend: exit %d\n%s", code, out)
	}
	if code, out := xoOut(t, "build", "-o", filepath.Join(t.TempDir(), "app"), app); code == 0 || !strings.Contains(out, "XO0900") {
		t.Errorf("build without --holes: exit %d\n%s", code, out)
	}
}
