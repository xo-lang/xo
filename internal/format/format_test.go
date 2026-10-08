package format

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
	"github.com/xo-lang/xo/internal/parser"
)

// shape is the syntax tree of a file as JSON without spans: two files with
// the same shape differ only in layout and comments.
func shape(t *testing.T, name string, src []byte, mode parser.Mode) (string, int) {
	t.Helper()
	f, ds := parser.ParseFile(name, src, mode)
	b, err := ast.ToJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	return spanRE.ReplaceAllString(string(b), ""), len(ds)
}

var spanRE = regexp.MustCompile(`"span": "[^"]*",?`)

// roundTrip formats src and checks that the result parses to the same tree
// and that formatting it again changes nothing.
func roundTrip(t *testing.T, name string, src []byte) []byte {
	t.Helper()
	out, err := Source(name, src)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return nil
	}
	mode := parser.Mode(0)
	if _, ds := parser.ParseFile(name, src, 0); len(ds) > 0 {
		mode = parser.SignatureMode
	}
	want, _ := shape(t, name, src, mode)
	got, n := shape(t, name, out, mode)
	if n > 0 {
		_, ds := parser.ParseFile(name, out, mode)
		t.Errorf("%s: formatted file does not parse: %v\n--- formatted ---\n%s", name, ds, out)
		return nil
	}
	if got != want {
		t.Errorf("%s: formatting changed the syntax tree\n%s\n--- formatted ---\n%s", name, firstDiff(want, got), out)
		return nil
	}
	again, err := Source(name, out)
	if err != nil {
		t.Errorf("%s: second format: %v", name, err)
		return nil
	}
	if !bytes.Equal(again, out) {
		t.Errorf("%s: formatting is not idempotent\n%s", name, firstDiff(string(out), string(again)))
	}
	if cs := commentTexts(src); !equalStrings(cs, commentTexts(out)) {
		t.Errorf("%s: comments changed", name)
	}
	return out
}

func commentTexts(src []byte) []string {
	var ds diag.List
	var out []string
	for _, c := range lexer.Lex("x.xo", src, &ds).Comments {
		out = append(out, strings.TrimRight(c.Text, " \t"))
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return fmt.Sprintf("line %d:\n  want %q\n  got  %q", i+1, al[i], bl[i])
		}
	}
	return fmt.Sprintf("length %d vs %d lines", len(al), len(bl))
}

func TestLayout(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"indent and spacing", "fn f(a:Int,b:Int)->Int{\n    let x=a+b\n  x*2\n}\n",
			"fn f(a: Int, b: Int) -> Int {\n  let x = a + b\n  x * 2\n}\n"},
		{"semicolons", "fn f() { let a = 1; let b = 2 }\n",
			"fn f() {\n  let a = 1\n  let b = 2\n}\n"},
		{"one line block kept", "fn f() -> Int { 1 }\n", "fn f() -> Int { 1 }\n"},
		{"blank lines collapse", "use std/http\n\n\n\nfn f() {\n\n  let a = 1\n\n\n  a\n\n}\n",
			"use std/http\n\nfn f() {\n  let a = 1\n\n  a\n}\n"},
		{"broken list gets trailing comma", "fn f() {\n  g(\n    1, 2,\n  )\n}\n",
			"fn f() {\n  g(\n    1,\n    2,\n  )\n}\n"},
		{"joined list drops trailing comma", "fn f() {\n  g(1, 2,)\n}\n", "fn f() {\n  g(1, 2)\n}\n"},
		{"else on the brace line", "fn f(a: Bool) -> Int {\n  if a {\n    1\n  } else {\n    2\n  }\n}\n",
			"fn f(a: Bool) -> Int {\n  if a {\n    1\n  } else {\n    2\n  }\n}\n"},
		{"trailing comments aligned", "fn f() {\n  let a = 1 // one\n  let bbb = 2   // two\n}\n",
			"fn f() {\n  let a = 1   // one\n  let bbb = 2 // two\n}\n"},
		{"comments kept", "// file\n\n/// Doc.\nfn f() {\n  // inside\n  g()\n  // end\n}\n",
			"// file\n\n/// Doc.\nfn f() {\n  // inside\n  g()\n  // end\n}\n"},
		{"method chain", "fn f() {\n  http.json(429, x)\n      .with_header(\"a\", b)\n}\n",
			"fn f() {\n  http.json(429, x)\n    .with_header(\"a\", b)\n}\n"},
		{"clauses", "pub fn f(x: Int) -> Int ! E\n    uses net\n    requires x > 0\n{\n  x\n}\n",
			"pub fn f(x: Int) -> Int ! E\n  uses net\n  requires x > 0\n{\n  x\n}\n"},
		{"one line struct", "struct P {x: Int,y: Int} invariant x >= 0 derive Json\n",
			"struct P { x: Int, y: Int } invariant x >= 0 derive Json\n"},
		{"struct fields", "pub struct P {\n  pub x: Int = 1\n    y: Str?\n}\n",
			"pub struct P {\n  pub x: Int = 1\n  y: Str?\n}\n"},
		{"match arms", "fn f(x: Int) -> Str {\n  match x {\n    1 | 2 => \"a\"\n    _ =>\n      \"b\"\n  }\n}\n",
			"fn f(x: Int) -> Str {\n  match x {\n    1 | 2 => \"a\"\n    _ =>\n      \"b\"\n  }\n}\n"},
		{"binary break", "fn f(a: Bool, b: Bool) -> Bool {\n  a &&\n        b\n}\n",
			"fn f(a: Bool, b: Bool) -> Bool {\n  a &&\n    b\n}\n"},
		{"optional fn type", "fn f(g: (fn() -> Int)?) {}\n", "fn f(g: (fn() -> Int)?) {}\n"},
		{"one tuple", "fn f() {\n  let t = (1,)\n}\n", "fn f() {\n  let t = (1,)\n}\n"},
		{"patterns", "fn f(xs: List[Int]) {\n  match xs {\n    [a,..,b] => ()\n    [..] => ()\n    _ => ()\n  }\n}\n",
			"fn f(xs: List[Int]) {\n  match xs {\n    [a, .., b] => ()\n    [..] => ()\n    _ => ()\n  }\n}\n"},
		{"multi line string kept", "fn f() -> Str {\n    \"\"\"\n      a  \n    b\n    \"\"\"\n}\n",
			"fn f() -> Str {\n  \"\"\"\n      a  \n    b\n    \"\"\"\n}\n"},
		{"interface one line", "interface Show { fn show(self) -> Str }\n", "interface Show { fn show(self) -> Str }\n"},
		{"enum", "enum E {\n  A\n  B(x: Int, from Err)\n} derive Display\n", "enum E {\n  A\n  B(x: Int, from Err)\n} derive Display\n"},
		{"test header", "test \"x\" for n: Int seed 3 {\n  expect(n == n)\n}\n", "test \"x\" for n: Int seed 3 {\n  expect(n == n)\n}\n"},
		{"comment in broken list", "fn f() {\n  g(\n    1, // one\n    // two\n    2,\n  )\n}\n",
			"fn f() {\n  g(\n    1, // one\n    // two\n    2,\n  )\n}\n"},
		{"comment in joined list moves out", "fn f() {\n  g(1, // one\n    2)\n  h()\n}\n",
			"fn f() {\n  g(1, 2)\n  // one\n  h()\n}\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Source("t.xo", []byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
			roundTrip(t, "t.xo", []byte(c.in))
		})
	}
}

func TestRejectsSyntaxErrors(t *testing.T) {
	_, err := Source("bad.xo", []byte("fn f( {\n"))
	if err == nil {
		t.Fatal("no error for a file that does not parse")
	}
	if _, ok := err.(*ParseError); !ok {
		t.Fatalf("error %T, want *ParseError", err)
	}
}

func TestSignatureMode(t *testing.T) {
	src := []byte("pub fn get(k: Str) -> Int?\npub fn put(var self: Map, k: Str)\n")
	out := roundTrip(t, "sig.xo", src)
	if string(out) != string(src) {
		t.Errorf("got:\n%s", out)
	}
}

// --------------------------------------------------------------------------
// Golden corpus: spec examples, testdata programs, bench solutions.

var fenceRE = regexp.MustCompile("^(\\s*)```(\\S*)\\s*$")

type fence struct {
	line int
	info string
	body string
}

func readFences(t *testing.T, path string) []fence {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []fence
	var cur *fence
	var indent string
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	n := 0
	for sc.Scan() {
		n++
		ln := sc.Text()
		m := fenceRE.FindStringSubmatch(ln)
		if cur == nil {
			if m != nil {
				cur = &fence{line: n, info: m[2]}
				indent = m[1]
				lines = nil
			}
			continue
		}
		if m != nil && m[2] == "" {
			cur.body = strings.Join(lines, "\n") + "\n"
			out = append(out, *cur)
			cur = nil
			continue
		}
		lines = append(lines, strings.TrimPrefix(ln, indent))
	}
	return out
}

// TestSpecExamples formats every xo example of spec/core.md that parses (as
// a file, as a function body, or as a signature listing).
func TestSpecExamples(t *testing.T) {
	n := 0
	for _, f := range readFences(t, "../../spec/core.md") {
		if f.info != "" && f.info != "xo" {
			continue
		}
		src := strings.ReplaceAll(f.body, "{ ... }", "{}")
		name := fmt.Sprintf("core.md:%d", f.line)
		if _, ds := parser.ParseFile(name, []byte(src), 0); len(ds) == 0 {
			roundTrip(t, name, []byte(src))
			n++
			continue
		}
		wrapped := "fn spec_example() {\n" + src + "}\n"
		if _, ds := parser.ParseFile(name, []byte(wrapped), 0); len(ds) == 0 {
			roundTrip(t, name, []byte(wrapped))
			n++
			continue
		}
		if _, ds := parser.ParseFile(name, []byte(src), parser.SignatureMode); len(ds) == 0 {
			roundTrip(t, name, []byte(src))
			n++
		}
	}
	if n < 30 {
		t.Errorf("only %d spec examples formatted", n)
	}
	t.Logf("%d spec examples formatted", n)
}

// corpusRoots are the program trees formatted by TestCorpus. Only temporary
// copies are formatted.
var corpusRoots = []string{
	"../../bench/xo",
	"../../bench/paper",
	"../../internal/check/testdata",
	"../../internal/gen/testdata",
	"../../std",
}

// TestCorpus formats a temporary copy of every testdata program, bench/xo
// solution, bench/paper solution, and std declaration file, and checks that
// each file parses to the same tree, that formatting is idempotent, and
// that every program type checks with the same diagnostics as before.
func TestCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("formats and type checks the whole corpus")
	}
	t.Setenv("XO_GOBIND_DIR", t.TempDir())
	tmp := t.TempDir()
	files, skipped := 0, 0
	var programs []string
	for _, root := range corpusRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel("../..", path)
			dst := filepath.Join(tmp, rel)
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && path != root {
					return filepath.SkipDir
				}
				return os.MkdirAll(dst, 0o755)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.HasSuffix(path, ".xo") {
				if _, ds := parser.ParseFile(path, data, 0); len(ds) > 0 {
					if _, ds := parser.ParseFile(path, data, parser.SignatureMode); len(ds) > 0 {
						skipped++ // old grammar (bench/paper/v0.1) or a planted syntax error
						return os.WriteFile(dst, data, 0o644)
					}
				}
				out := roundTrip(t, rel, data)
				files++
				if out != nil {
					data = out
				}
			}
			if d.Name() == "xo.mod" || (strings.HasSuffix(path, ".xo") && filepath.Base(filepath.Dir(path)) != "std") {
				programs = append(programs, filepath.Dir(rel))
			}
			return os.WriteFile(dst, data, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%d files formatted, %d skipped (syntax errors)", files, skipped)
	if files < 150 {
		t.Errorf("only %d files formatted", files)
	}
	// Type check each program directory before and after.
	seen := map[string]bool{}
	checked := 0
	for _, dir := range programs {
		top := programDir(dir)
		if seen[top] || strings.HasPrefix(top, "std") {
			continue
		}
		seen[top] = true
		before := checkSummary(t, filepath.Join("../..", top))
		after := checkSummary(t, filepath.Join(tmp, top))
		if before != after {
			t.Errorf("%s: diagnostics changed by formatting\n%s", top, firstDiff(before, after))
		}
		checked++
	}
	t.Logf("%d programs type checked before and after", checked)
}

// programDir maps a directory to the program it belongs to: bench task
// directories, golden case directories, or the directory itself.
func programDir(dir string) string {
	parts := strings.Split(filepath.ToSlash(dir), "/")
	n := len(parts)
	switch {
	case parts[0] == "bench" && n >= 4:
		n = 4 // bench/<arm>/<version>/<task>
	case parts[0] == "internal" && n >= 5 && parts[3] == "golden":
		n = 5 // internal/<pkg>/testdata/golden/<case>
	case parts[0] == "internal" && n >= 4:
		n = 4
	}
	return strings.Join(parts[:n], "/")
}

func checkSummary(t *testing.T, dir string) string {
	t.Helper()
	res, err := check.Check([]string{dir})
	if err != nil {
		return "error: " + err.Error()
	}
	var lines []string
	for _, d := range res.Diagnostics {
		lines = append(lines, fmt.Sprintf("%s %s %s", d.Code, d.Severity, d.Message))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

// TestGolden formats testdata/*.xo (every syntax form, comments in odd
// places) and compares with testdata/*.golden.
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.xo")
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out := roundTrip(t, f, src)
		golden := strings.TrimSuffix(f, ".xo") + ".golden"
		if *update {
			if err := os.WriteFile(golden, out, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("%s: output differs from %s\n%s", f, golden, firstDiff(string(want), string(out)))
		}
	}
}
