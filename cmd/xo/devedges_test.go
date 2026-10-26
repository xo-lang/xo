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
	// a body of a std implementation file is checked too (the preload of
	// native/NAME for std/NAME.xo, without which most of what a corpus
	// calls has no site at all): std/native/prelude.xo holds the sites of
	// the prelude methods this program uses
	hitImpl := false
	for _, f := range plain {
		if f[0] == "std/native/prelude.xo" {
			hitImpl = true
		}
	}
	if !hitImpl {
		t.Errorf("xo dev edges %s: no site in std/native/prelude.xo, so the implementation file's bodies were not checked", dir)
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

// TestDevEdgesMissingBinding: a program whose `use go` package has no
// binding stops the check before names resolve, and `xo dev edges` must
// exit 1 naming the reason instead of writing only the std references. A
// sweep reads this file per program directory and cannot tell a silent
// std-only dump from a program with no references of its own (the
// 2026-10-26 coverage sweep lost every `use go` golden that way, and
// go.Error.go_type looked unreached although golden/go_url calls it).
func TestDevEdgesMissingBinding(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module nobind\n",
		"main.xo": "use go \"net/url\"\n\nfn main(os: Os) -> () {\n  os.stdio.println(\"x\")\n}\n",
	})
	var out, errOut bytes.Buffer
	args := []string{"dev", "edges", dir}
	if code := run(t, args, &out, &errOut); code == 0 {
		t.Fatalf("xo dev edges wrote %d lines, all std, and exited 0; want exit 1 naming the missing binding\n--- stderr\n%s", len(strings.Split(strings.TrimSpace(out.String()), "\n")), errOut.String())
	}
	if !strings.Contains(errOut.String(), "net/url") {
		t.Errorf("the reason does not name the package: %q", errOut.String())
	}
}

// TestDevEdgesMissingBindingKeepsOtherCommands: the edge list refuses a
// program that did not check, and nothing else does. `xo def`, `xo refs`,
// `xo doc`, `xo query` and `xo symbols` answer from std and say that the
// check stopped, which is what tells a reader the answer is partial; the
// sweep fix must not turn them into failures (reviewer, 2026-10-26).
func TestDevEdgesMissingBindingKeepsOtherCommands(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module nobind\n",
		"main.xo": "use go \"net/url\"\n\nfn main(os: Os) -> () {\n  os.stdio.println(\"x\")\n}\n",
	})
	for _, args := range [][]string{
		{"def", "--dir=" + dir, "parse"},
		{"refs", "--dir=" + dir, "parse"},
		{"doc", "--dir=" + dir, "parse"},
	} {
		var out, errOut bytes.Buffer
		code := run(t, args, &out, &errOut)
		text := out.String() + errOut.String()
		if !strings.Contains(text, "the check stopped before resolving names") {
			t.Errorf("xo %s: no stopped-check explanation: %q", strings.Join(args, " "), text)
		}
		if !strings.Contains(text, "net/url") {
			t.Errorf("xo %s: the answer does not name the missing binding: %q", strings.Join(args, " "), text)
		}
		if code != 1 {
			t.Errorf("xo %s: exit %d, want 1 (the name does not resolve)", strings.Join(args, " "), code)
		}
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
