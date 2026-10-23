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

// TestHoleInInterpolation: a hole interpolated into a string (`"${???}"`,
// and `"${???:?}"`) has the type an interpolated value becomes, Str, with
// the values in scope that fit it (field report 0.26.0: it said "hole of
// type unknown" and listed none).
func TestHoleInInterpolation(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn greet(name: Str) -> Str {\n  \"hello ${???}\"\n}\n\nfn debug(name: Str) -> Str {\n  \"got ${???:?}\"\n}\n\nfn main(os: Os) {\n  os.stdio.println(greet(\"x\") + debug(\"y\"))\n}\n",
	})
	var out, errb strings.Builder
	run(t, []string{"check", "--json", filepath.Join(dir, "app")}, &out, &errb)
	text := out.String()
	if strings.Count(text, "hole of type `Str`") != 2 || strings.Contains(text, "hole of type unknown") {
		t.Fatalf("want two holes of type `Str`:\n%s%s", text, errb.String())
	}
	if !strings.Contains(text, "in scope values that fit: `name`") {
		t.Errorf("no note naming `name`, the Str in scope:\n%s", text)
	}
}
