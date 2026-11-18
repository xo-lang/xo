package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStdVariantLineWholeProgram (task #298, lead decision #115): a
// build that the size and instruction-count tools make (XO_SEPCOMP=0:
// xo dev size, scripts/identity, ab-host's program builds, the HTTP
// callgrind run) and a --release build each say with --phase-times that
// the whole program is built. Before, neither printed a std variant
// line, so a measurement could not show which pipeline it compared
// (gotchas/separate-std-changes-code-size.md: compiler/xo 37.00 MB of
// code linking std's store against 37.96 MB whole).
func TestStdVariantLineWholeProgram(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	setenv(t, "XO_WHOLE_BUILD", "off")
	dir := filepath.Join(tmp, "prog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"xo.mod":  "module sepcomppin\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	for _, c := range []struct {
		name string
		env  string
		args []string
		want string
	}{
		// --release first: setenv has no unset, and XO_SEPCOMP stays set
		{"--release", "", []string{"--release"}, "std variant: not used (--release), the whole program is built"},
		{"XO_SEPCOMP=0", "0", nil, "std variant: off (XO_SEPCOMP=0 or XO_SEPCOMP_DEFAULT=0), the whole program is built"},
	} {
		if c.env != "" {
			setenv(t, "XO_SEPCOMP", c.env)
		}
		var so, se bytes.Buffer
		args := append([]string{"build", "--phase-times", "-o", filepath.Join(dir, "prog")}, c.args...)
		if code := run(t, append(args, "."), &so, &se); code != 0 {
			t.Fatalf("%s: build: exit %d\n%s%s", c.name, code, so.String(), se.String())
		}
		if !strings.Contains(se.String(), c.want) {
			t.Errorf("%s: no line %q in the build's --phase-times output:\n%s", c.name, c.want, grepStd(se.String()))
		}
	}
}

func grepStd(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "std") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
