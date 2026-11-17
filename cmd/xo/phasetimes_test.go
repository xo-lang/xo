package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestPhaseTimesParseSafe: each timed --phase-times line is
// "phase NAME MS ms heap_mb=N peak_mb=N" (task #179). The line ended
// "MS ms (heap N MB, peak N MB)" until 2026-11-16, and an awk $(NF-1)
// read a heap figure as the phase's milliseconds (twice on 2026-10-27,
// once for a whole A/B's column). Now the time is field 3 and no later
// field is a bare number.
func TestPhaseTimesParseSafe(t *testing.T) {
	t.Parallel()
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
		"xo.mod":  "module phasetimes\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	var so, se bytes.Buffer
	if code := run(t, []string{"build", "--phase-times", "-o", filepath.Join(dir, "prog"), "."}, &so, &se); code != 0 {
		t.Fatalf("build: exit %d\n%s%s", code, so.String(), se.String())
	}
	timed := regexp.MustCompile(`^phase \S+ [0-9]+(\.[0-9]+)? ms`)
	want := regexp.MustCompile(`^phase \S+ [0-9]+(\.[0-9]+)? ms heap_mb=[0-9]+ peak_mb=[0-9]+$`)
	bare := regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
	n := 0
	for _, l := range strings.Split(se.String(), "\n") {
		if !timed.MatchString(l) {
			continue
		}
		n++
		if !want.MatchString(l) {
			t.Errorf("phase line not in the parse safe layout (phase NAME MS ms heap_mb=N peak_mb=N):\n%s", l)
			continue
		}
		f := strings.Fields(l)
		for _, x := range f[4:] {
			if bare.MatchString(x) {
				t.Errorf("a bare number %q after the time in %q", x, l)
			}
		}
	}
	if n == 0 {
		t.Fatalf("no timed phase lines in:\n%s", se.String())
	}
}
