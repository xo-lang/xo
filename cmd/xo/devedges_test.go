package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestDevEdges: `xo dev edges` writes one tab separated line per resolved
// reference of a checked program (`xo dev edges DIR`: SRC_PATH SRC_LINE
// SRC_NAME TGT_PATH TGT_LINE TGT_NAME SITE_PATH SITE_LINE SITE_COL
// SITE_NAME), and --impls adds the edges from an interface method to the
// methods that implement it, which the checker's reference resolution
// alone does not name.
func TestDevEdges(t *testing.T) {
	dir := filepath.Join(repoRoot, "internal", "llvm", "testdata", "golden", "strwhere")
	plain := devEdges(t, dir)
	if len(plain) < 100 {
		t.Fatalf("xo dev edges %s: %d lines, want the program's references", dir, len(plain))
	}
	// the golden calls s.contains_where(f), resolved to the prelude
	// method, never named in the text as a declaration
	hit := false
	for _, f := range plain {
		if f[9] == "contains_where" && f[3] == "std/prelude.xo" {
			hit = true
		}
	}
	if !hit {
		t.Errorf("xo dev edges %s: no edge resolves a contains_where call to std/prelude.xo", dir)
	}
	impls := devEdges(t, dir, "--impls")
	extra := 0
	// an interface method of the prelude, reached through the interface,
	// with the implementation it dispatches to
	hit = false
	seen := map[string]bool{}
	for _, f := range plain {
		seen[strings.Join(f, "\t")] = true
	}
	for _, f := range impls {
		if seen[strings.Join(f, "\t")] {
			continue
		}
		extra++
		if f[0] == "std/prelude.xo" && f[3] != "" && f[2] != f[5] {
			hit = true
		}
	}
	if extra == 0 {
		t.Errorf("xo dev edges --impls %s: no implementation edges", dir)
	}
	if !hit {
		t.Errorf("xo dev edges --impls %s: no edge from a prelude interface method to an implementation", dir)
	}
}

// devEdges runs `xo dev edges [extra...] DIR` and splits its output into
// tab separated fields, checking the field count on the way.
func devEdges(t *testing.T, dir string, extra ...string) [][]string {
	t.Helper()
	args := append([]string{"dev", "edges"}, extra...)
	args = append(args, dir)
	var out, errOut bytes.Buffer
	if code := run(t, args, &out, &errOut); code != 0 {
		t.Fatalf("xo %s: exit %d: %s", strings.Join(args, " "), code, errOut.String())
	}
	text := strings.TrimSuffix(out.String(), "\n")
	if text == "" {
		return nil
	}
	var lines [][]string
	for _, l := range strings.Split(text, "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 10 {
			t.Fatalf("xo dev edges: %d fields, want 10: %q", len(f), l)
		}
		lines = append(lines, f)
	}
	return lines
}
