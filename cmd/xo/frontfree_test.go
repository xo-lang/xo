package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFrontEndFreedByDefault: a native build lets the front end go once
// the SSA is built (decision 0352), under the module check cache's auto
// mode too, which is the default. It was off for three days from
// 2026-10-23 (strip required `!c.mod_skip_auto` and auto became the
// default): every default build kept about 130 MB of parse trees and
// checker data through the passes. --phase-times prints the phase
// front_end_freed where the front end goes; XO_KEEP_FRONT=1 keeps it.
func TestFrontEndFreedByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	// no whole build hit: the phases must run
	setenv(t, "XO_WHOLE_BUILD", "off")
	dir := filepath.Join(tmp, "prog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"xo.mod":  "module frontfree\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	phases := func() string {
		t.Helper()
		var so, se bytes.Buffer
		code := run(t, []string{"build", "--phase-times", "-o", filepath.Join(dir, "prog"), "."}, &so, &se)
		if code != 0 {
			t.Fatalf("build: exit %d\n%s%s", code, so.String(), se.String())
		}
		return se.String()
	}
	// the default, twice (the second reads the caches: the module check
	// cache's auto mode is then in play)
	for i, what := range []string{"first build", "second build"} {
		if se := phases(); !strings.Contains(se, "phase front_end_freed ") {
			t.Fatalf("%s (%d): no phase front_end_freed in the default build's phase times:\n%s", what, i, se)
		}
	}
	setenv(t, "XO_KEEP_FRONT", "1")
	if se := phases(); strings.Contains(se, "phase front_end_freed ") {
		t.Fatalf("XO_KEEP_FRONT=1 freed the front end:\n%s", se)
	}
}
