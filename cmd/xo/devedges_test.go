package main

import (
	"bytes"
	"os"
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
	t.Parallel()
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

// TestDevEdgesOwnStd: `xo dev edges --own` prints a `# bindings B` line
// and the program's own edges (no site in the std tree), `--std` the std
// graph alone (every site in it), and the two together are refused: a
// sweep over many programs (internal/stdcoverage.Sweep) reads --own of
// each and --std once a bindings value, where the full output repeated
// the std graph with every program (47 GB over 1,381 programs). The
// program uses a C header bound by `xo check` first, into $XO_CBIND_DIR,
// which the tests set (internal/xotool): the edge list's checker reads
// that cache too, where it read only the module's .xo/cbind and stopped
// on "use c without the binding" that the check had just written
// (2026-11-11).
func TestDevEdgesOwnStd(t *testing.T) {
	if os.Getenv("XO_CBIND_DIR") == "" {
		t.Skip("needs XO_CBIND_DIR, which internal/xotool sets for the tests")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module ownstd\n",
		"addc.h":  "#include <stdint.h>\nint64_t addc_add(int64_t a, int64_t b);\n",
		"addc.c":  "#include \"addc.h\"\nint64_t addc_add(int64_t a, int64_t b) { return a + b; }\n",
		"main.xo": "use c \"addc.h\" link \"addc.c\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${addc.addc_add(40, 2)}\".to_upper())\n}\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo check %s: exit %d: %s%s", dir, code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"dev", "edges", "--own", dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo dev edges --own after xo check bound the header: exit %d: %s", code, errOut.String())
	}
	first, rest, _ := strings.Cut(out.String(), "\n")
	if first != "# bindings c" {
		t.Errorf("xo dev edges --own: first line %q, want \"# bindings c\"", first)
	}
	own := 0
	for _, l := range strings.Split(strings.TrimSpace(rest), "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 10 {
			t.Fatalf("xo dev edges --own: %d fields, want 10: %q", len(f), l)
		}
		if strings.HasPrefix(f[6], "std/") {
			t.Errorf("xo dev edges --own: a site in the std tree: %q", l)
		}
		own++
	}
	// the program's own calls: addc_add, Str.to_upper and println at least
	if own < 3 {
		t.Errorf("xo dev edges --own: %d edges, want the program's own calls", own)
	}
	std := devEdges(t, dir, "--std")
	if len(std) < 100 {
		t.Errorf("xo dev edges --std: %d edges, want the std graph", len(std))
	}
	for _, f := range std {
		if !strings.HasPrefix(f[6], "std/") {
			t.Errorf("xo dev edges --std: a site outside the std tree: %q", strings.Join(f, "\t"))
			break
		}
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"dev", "edges", "--own", "--std", dir}, &out, &errOut); code != 2 {
		t.Errorf("xo dev edges --own --std: exit %d, want 2 (one or the other)", code)
	}
}

// TestDevEdgesStdGraphSameForEveryProgram: the std graph (`--std`, what
// the edge cache keeps once for every program with the same bindings)
// holds the bodies of a merged std module, one whose bodies are in
// std/NAME.xo with no std/native twin (std/sse.xo), even when the program
// does not import it. Before, only a program importing sse checked its
// bodies, so the cached graph had them or not by which program filled the
// entry, and the resolved coverage sweep read sse.source as unreached
// (2026-11-11).
func TestDevEdgesStdGraphSameForEveryProgram(t *testing.T) {
	t.Setenv("XO_EDGES_CACHE", "off")
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module nosse\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"x\")\n}\n",
	})
	hit := false
	for _, f := range devEdges(t, dir, "--std") {
		if f[6] == "std/sse.xo" && f[2] == "subscribe" && f[5] == "source" {
			hit = true
		}
	}
	if !hit {
		t.Errorf("xo dev edges --std of a program without `use std/sse`: no edge from sse.subscribe's body to sse.source")
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

// TestDevEdgesCache: the resolved edge cache (decision 0381). `xo dev
// edges` prints the same lines with the cache off (XO_EDGES_CACHE=off:
// std's bodies checked with the program), on a miss (the std graph
// stored as one entry under <backend cache>/edges), on a hit (the program
// alone checked, its own edges with the cached graph) and under
// XO_BACKEND_CACHE=verify (both checks, compared with the entry); the
// output has the program's own sites and std's.
func TestDevEdgesCache(t *testing.T) {
	if testing.Short() {
		t.Skip("checks std's bodies three times, about 16 s each")
	}
	cache := filepath.Join(t.TempDir(), "backend")
	setenv(t, "XO_BACKEND_CACHE_DIR", cache)
	setenv(t, "XO_CACHE_STATS", "1")
	dir := filepath.Join(repoRoot, "internal", "llvm", "testdata", "golden", "strwhere")
	edges := func(mode, backend, want string) string {
		t.Helper()
		setenv(t, "XO_EDGES_CACHE", mode)
		setenv(t, "XO_BACKEND_CACHE", backend)
		var out, errOut bytes.Buffer
		args := []string{"dev", "edges", dir}
		if code := run(t, args, &out, &errOut); code != 0 {
			t.Fatalf("xo dev edges (XO_EDGES_CACHE=%s XO_BACKEND_CACHE=%s): exit %d: %s", mode, backend, code, errOut.String())
		}
		if !strings.Contains(errOut.String(), want) || strings.Contains(errOut.String(), "differ") {
			t.Errorf("xo dev edges (XO_EDGES_CACHE=%s XO_BACKEND_CACHE=%s): stderr %q, want %q and no difference", mode, backend, errOut.String(), want)
		}
		return out.String()
	}
	off := edges("off", "", "cache edges off")
	if len(off) < 1000 || !strings.Contains(off, "\tstd/native/prelude.xo\t") || !strings.Contains(off, "\t"+filepath.ToSlash(filepath.Join(dir, "main.xo"))+"\t") {
		t.Fatalf("the uncached edges lack the program's or std's sites (%d bytes)", len(off))
	}
	miss := edges("", "", "cache edges miss (none, ")
	entries, err := filepath.Glob(filepath.Join(cache, "edges", "*"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("after a miss: %d entries under %s, want 1 (%v)", len(entries), filepath.Join(cache, "edges"), err)
	}
	hit := edges("", "", "cache edges hit (none)")
	verify := edges("", "verify", "cache edges verify (none, ")
	for _, c := range []struct{ what, got string }{{"miss", miss}, {"hit", hit}, {"verify", verify}} {
		if c.got != off {
			a, b := strings.Split(off, "\n"), strings.Split(c.got, "\n")
			only := 0
			seen := map[string]bool{}
			for _, l := range a {
				seen[l] = true
			}
			for _, l := range b {
				if !seen[l] {
					only++
				}
			}
			t.Errorf("the %s output differs from the uncached one: %d against %d lines, %d lines only in the %s output", c.what, len(a), len(b), only, c.what)
		}
	}
}
