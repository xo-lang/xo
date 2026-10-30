package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xo-lang/xo/internal/testtmp"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestFmt(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"a.xo":          "fn f(a:Int)->Int{\n    a+1\n}\n",
		"b.xo":          "fn g() -> Int { 2 }\n",
		".xo/skip.xo":   "fn  skipped( ) {}\n",
		"sub/c.xo":      "fn h() {\n  let x = 1;let y = 2\n}\n",
		"notes/read.md": "not xo\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"fmt", "--check", dir + "/..."}, &out, &errOut); code != 1 {
		t.Fatalf("--check exit %d, want 1: %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "a.xo") || !strings.Contains(got, "c.xo") || strings.Contains(got, "b.xo") || strings.Contains(got, "skip.xo") {
		t.Errorf("--check listed:\n%s", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.xo")); string(data) != "fn f(a:Int)->Int{\n    a+1\n}\n" {
		t.Errorf("--check changed a file")
	}
	out.Reset()
	if code := run(t, []string{"fmt", dir + "/..."}, &out, &errOut); code != 0 {
		t.Fatalf("fmt exit %d: %s", code, errOut.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.xo")); string(data) != "fn f(a: Int) -> Int {\n  a + 1\n}\n" {
		t.Errorf("a.xo = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".xo/skip.xo")); string(data) != "fn  skipped( ) {}\n" {
		t.Errorf("dot directory was formatted")
	}
	out.Reset()
	if code := run(t, []string{"fmt", "--check", dir + "/..."}, &out, &errOut); code != 0 || out.Len() != 0 {
		t.Errorf("second --check: exit %d, out %q", code, out.String())
	}
}

// TestFmtCoalesceArm: `r ?? e => body` (decision 0321) prints back in
// its one line form, not as the match the parser makes of it.
func TestFmtCoalesceArm(t *testing.T) {
	t.Parallel()
	src := "fn f(r: Result[Int, Str]) -> Result[Int, Str] {\n  let v = r ??  e=>return Err(e)\n  let w = r ?? _ =>  {\n    0\n  }\n  Ok(v + w)\n}\n"
	want := "fn f(r: Result[Int, Str]) -> Result[Int, Str] {\n  let v = r ?? e => return Err(e)\n  let w = r ?? _ => {\n    0\n  }\n  Ok(v + w)\n}\n"
	dir := writeFiles(t, map[string]string{"a.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"fmt", dir}, &out, &errOut); code != 0 {
		t.Fatalf("fmt exit %d: %s", code, errOut.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.xo")); string(data) != want {
		t.Errorf("a.xo = %q, want %q", data, want)
	}
}

func TestFmtSyntaxError(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"bad.xo": "fn f( {\n", "ok.xo": "fn g()  {}\n"})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"fmt", "--json", dir}, &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	var res struct {
		Files []struct {
			Path    string `json:"path"`
			Changed bool   `json:"changed"`
		} `json:"files"`
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if len(res.Files) != 1 || !res.Files[0].Changed || len(res.Diagnostics) == 0 {
		t.Errorf("result = %+v", res)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "bad.xo")); string(data) != "fn f( {\n" {
		t.Errorf("file with syntax errors was rewritten")
	}
}

const toolsProg = `/// A counter.
///
/// ` + "```" + `
/// let c = Counter{n: 1}
/// ` + "```" + `
pub struct Counter {
  pub n: Int
  hidden: Int = 0
}

/// Adds one.
pub fn (c: Counter) next() -> Counter {
  c with {n: c.n + 1}
}

/// Reads a file.
pub fn load(f: Fs, p: Str) -> Str ! Error uses fs.read, clock {
  read_it(f, p)?
}

fn read_it(f: Fs, p: Str) -> Str ! Error {
  f.read_text(p)?
}

pub fn later() -> Int {
  ???
}

pub fn twice(c: Counter) -> Counter {
  let m = c.next()
  dbg(m.next())
}

pub struct Wide {
  a: Str, b: Str, c: Str, d: Str
  e: Str, f: Str, g: List[Int], h: Map[Str, Str]
}

pub struct Holder {
  w: Wide?
}

pub fn first(h: Holder) -> Str {
  match h.w {
    Some(w) => w.a
    None => ""
  }
}

pub struct Box {
  w: Wide
}

fn keep(w: Wide) -> Box {
  Box{w: w}
}

pub fn rebox(b: Box) -> Box {
  let w = b.w
  keep(w)
}
`

func TestVetCLI(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": toolsProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	got := map[string]bool{}
	for _, d := range res.Diagnostics {
		got[d.Code] = true
		if d.Severity == "error" {
			t.Errorf("vet reported an error: %+v", d)
		}
	}
	for _, c := range []string{"XO0530", "XO0531", "XO0703", "XO0900", "XO0901"} {
		if !got[c] {
			t.Errorf("missing %s in %s", c, out.String())
		}
	}
	if res.CheckErrors != 0 {
		t.Errorf("check errors: %d", res.CheckErrors)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"vet", dir}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "warning XO0703") || !strings.Contains(errOut.String(), "1 module(s)") {
		t.Errorf("human output: exit %d\n%s\n%s", code, out.String(), errOut.String())
	}
}

// TestVetVerifyNone: XO0902 (decision 0281) flags tls.Verify.None, once
// per use, and nothing else named None (a program's own enum, Option).
func TestVetVerifyNone(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": `use std/tls

enum Mode {
  On
  None
} derive Display

fn pick(strict: Bool) -> tls.Verify {
  if strict {
    return tls.Verify.ChainOnly
  }
  tls.Verify.None
}

fn main(os: Os) {
  let m = Mode.None
  let o: Int? = None
  os.stdio.println("${pick(false)} ${m} ${o ?? 0}")
}
`})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0902" {
			lines = append(lines, d.Span.Line)
		}
	}
	if res.CheckErrors != 0 {
		t.Errorf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	if len(lines) != 1 || lines[0] != 12 {
		t.Errorf("XO0902 at lines %v, want [12]\n%s", lines, out.String())
	}
}

// copyStoreProg has the shape of decision 0266, a copy changed and
// stored back (XO0532, line 4), and three that are not: a store with
// another statement between, a copy that is only read, and the update
// form the fix writes. (A change never stored back is the checker's
// error XO0520, so a program that vets has none.)
const copyStoreProg = `fn index(var by: Map[Str, List[Int]], name: Str, i: Int) {
  let seen = 1
  let _ = seen
  var ids = by.get(name) ?? []
  ids.push(i)
  by[name] = ids
}

fn count(by: Map[Str, List[Int]], name: Str) -> Int {
  let ids = by.get(name) ?? []
  ids.len()
}

fn gap(var by: Map[Str, List[Int]], name: Str, var n: Int) {
  var ids = by.get(name) ?? []
  ids.push(1)
  n += 1
  by[name] = ids
}

fn fixed(var by: Map[Str, List[Int]], name: Str, i: Int) {
  by.update(name, [], fn(var ids) {
    ids.push(i)
  })
}

fn main(os: Os) {
  var by: Map[Str, List[Int]] = {}
  index(by, "a", 1)
  var n = 0
  gap(by, "a", n)
  fixed(by, "a", 2)
  os.stdio.println("${count(by, "a")} ${by.len()}")
}
`

func TestVetCopyStore(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": copyStoreProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Span    struct {
				Line int `json:"line"`
			} `json:"span"`
			Fixes []struct {
				Edits []struct {
					Replace string `json:"replace"`
				} `json:"edits"`
			} `json:"fixes"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if res.CheckErrors != 0 {
		t.Fatalf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	var got []string
	for _, d := range res.Diagnostics {
		if d.Code != "XO0532" {
			continue
		}
		got = append(got, fmt.Sprintf("%s %d", d.Code, d.Span.Line))
		if d.Code == "XO0532" {
			want := "by.update(name, [], fn(var ids) {\n    ids.push(i)\n  })"
			if len(d.Fixes) != 1 || len(d.Fixes[0].Edits) != 1 || d.Fixes[0].Edits[0].Replace != want {
				t.Errorf("XO0532 fix: %+v, want %q", d.Fixes, want)
			}
		}
	}
	if strings.Join(got, ", ") != "XO0532 4" {
		t.Errorf("got %v\n%s", got, out.String())
	}
}

func queryJSON(t *testing.T, dir string, args ...string) map[string]any {
	t.Helper()
	var out, errOut bytes.Buffer
	args = append([]string{"query"}, args...)
	args = append(args, "--dir="+dir)
	if code := run(t, args, &out, &errOut); code != 0 {
		t.Fatalf("%v: exit %d: %s", args, code, errOut.String())
	}
	var v any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%v: not JSON (JSON is the default off a terminal): %v\n%s", args, err, out.String())
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{"list": v}
}

func TestQueryCLI(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": toolsProg})
	file := filepath.Join(dir, "main.xo")

	ty := queryJSON(t, dir, "type", file+":30:7")
	if ty["type"] != "Counter" || ty["expr"] != "m" {
		t.Errorf("type = %v", ty)
	}

	def := queryJSON(t, dir, "def", "Counter.next")["list"].([]any)
	if len(def) != 1 || !strings.Contains(def[0].(map[string]any)["signature"].(string), "pub fn (c: Counter) next() -> Counter") {
		t.Errorf("def = %v", def)
	}
	if d := queryJSON(t, dir, "def", "http.client")["list"].([]any); len(d) != 1 {
		t.Errorf("def http.client = %v", d)
	}

	callers := queryJSON(t, dir, "callers", "Counter.next")["callers"].([]any)
	if len(callers) != 2 {
		t.Errorf("callers = %v", callers)
	}

	eff := queryJSON(t, dir, "effects", "load")
	perf := eff["performed"].([]any)
	if len(perf) != 1 {
		t.Fatalf("effects = %v", eff)
	}
	p0 := perf[0].(map[string]any)
	chain := p0["chain"].([]any)
	if p0["effect"] != "fs.read" || len(chain) != 2 || !strings.Contains(chain[0].(map[string]any)["message"].(string), "read_it") {
		t.Errorf("effects = %v", eff)
	}

	holes := queryJSON(t, dir, "holes")["list"].([]any)
	if len(holes) != 1 {
		t.Errorf("holes = %v", holes)
	}

	api := queryJSON(t, dir, "api", dir)
	items := api["items"].([]any)
	var sigs []string
	for _, it := range items {
		sigs = append(sigs, it.(map[string]any)["signature"].(string))
	}
	all := strings.Join(sigs, "\n")
	if strings.Contains(all, "hidden") || strings.Contains(all, "read_it") || !strings.Contains(all, "pub fn load") || !strings.Contains(all, "pub n: Int") {
		t.Errorf("api:\n%s", all)
	}

	imps := queryJSON(t, dir, "imports", dir)
	if _, ok := imps["imports"].([]any); !ok || imps["transitive"] != false { // the program has no use lines
		t.Errorf("imports = %v", imps)
	}
	if deps := queryJSON(t, dir, "deps", dir); deps["transitive"] != true {
		t.Errorf("deps = %v", deps)
	}

	var out, errOut bytes.Buffer
	if code := run(t, []string{"query", "callers", "nosuch", "--dir=" + dir}, &out, &errOut); code != 1 {
		t.Errorf("unknown name: exit %d", code)
	}
	if code := run(t, []string{"query", "bogus", "x"}, &out, &errOut); code != 2 {
		t.Errorf("unknown kind: exit %d", code)
	}
	out.Reset()
	if code := run(t, []string{"query", "effects", "load", "--text", "--dir=" + dir}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "performs fs.read") {
		t.Errorf("text output: %s", out.String())
	}
}

func TestDocCLI(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": toolsProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"doc", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	md := out.String()
	for _, want := range []string{"# Module `", "### Counter", "#### Counter.next", "Adds one.", "```xo\nlet c = Counter{n: 1}\n```", "1 example(s)", "### load"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "read_it") || strings.Contains(md, "hidden") {
		t.Errorf("non pub declarations documented:\n%s", md)
	}
	out.Reset()
	if code := run(t, []string{"doc", "--all", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"read_it"`) || !strings.Contains(out.String(), `"examples": [`) {
		t.Errorf("--all --json:\n%s", out.String())
	}
}

func TestExplainCLI(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run(t, []string{"explain", "xo0201"}, &out, &errOut); code != 0 || !strings.HasPrefix(out.String(), "XO0201: ") || !strings.Contains(out.String(), "Fix:") {
		t.Errorf("exit %d:\n%s", code, out.String())
	}
	out.Reset()
	if code := run(t, []string{"explain", "--json", "XO0703"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var e map[string]string
	if err := json.Unmarshal(out.Bytes(), &e); err != nil || e["severity"] != "warning" || e["example"] == "" {
		t.Errorf("json = %v (%v)", e, err)
	}
	if code := run(t, []string{"explain", "XO9999"}, &out, &errOut); code != 1 {
		t.Errorf("unknown code: exit %d", code)
	}
	if code := run(t, []string{"explain"}, &out, &errOut); code != 2 {
		t.Errorf("no code: exit %d", code)
	}
}

// agentJSON runs one of decision 0261's agent commands and decodes its
// JSON; code is the exit status wanted.
// xoStd is the std the xo under test reads: $XO_STD, else a release
// layout's lib/xo/std beside its bin/ (release-verify.sh runs an archive's
// xo with no XO_STD, and a std elsewhere is an ordinary directory to it),
// else the checkout's.
func xoStd() string {
	if s := os.Getenv("XO_STD"); s != "" {
		return s
	}
	if cli, err := xoCommand(); err == nil && len(cli) > 0 {
		s := filepath.Join(filepath.Dir(cli[0]), "..", "lib", "xo", "std")
		if st, err := os.Stat(s); err == nil && st.IsDir() {
			return filepath.Clean(s)
		}
	}
	return filepath.Join(repoRoot, "std")
}

func agentJSON(t *testing.T, code int, args ...string) map[string]any {
	t.Helper()
	// text is the default (decision 0386): ask for JSON
	args = append(append([]string{}, args...), "--json")
	var out, errOut bytes.Buffer
	if got := run(t, args, &out, &errOut); got != code {
		t.Fatalf("%v: exit %d, want %d: %s%s", args, got, code, out.String(), errOut.String())
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%v: not JSON: %v\n%s", args, err, out.String())
	}
	return v
}

// TestDefMissInStdNative: a name declared only in a std/native file, which
// a module does not load, gets the directory to search, not a "did you
// mean" of the same name (2026-10-27: `xo def Printer.pop_prefix` from the
// repository's root suggested Printer.pop_prefix).
func TestDefMissInStdNative(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": toolsProg})
	var out, errb bytes.Buffer
	if code := runXo(t, []string{"def", "--dir=" + dir, "Printer.write_indent"}, &out, &errb); code != 1 {
		t.Fatalf("exit %d, want 1\n%s%s", code, out.String(), errb.String())
	}
	e := errb.String()
	if !strings.Contains(e, "`Printer.write_indent` is declared in std/native/xml.xo:") || !strings.Contains(e, "--dir=std/native") || strings.Contains(e, "did you mean") {
		t.Errorf("stderr = %q", e)
	}
}

// TestAgentCLI covers xo def, refs, type, doc NAME, and symbols (decision
// 0261 stage (a)) on toolsProg.
func TestAgentCLI(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": toolsProg})
	file := filepath.Join(dir, "main.xo")
	d := "--dir=" + dir

	decls := agentJSON(t, 0, "def", d, "Counter.next")["declarations"].([]any)
	if len(decls) != 1 {
		t.Fatalf("def Counter.next = %v", decls)
	}
	d0 := decls[0].(map[string]any)
	if d0["kind"] != "func" || !strings.Contains(d0["signature"].(string), "next() -> Counter") || d0["line"].(float64) < 1 {
		t.Errorf("def Counter.next = %v", d0)
	}
	// a short name finds the method too
	if ds := agentJSON(t, 0, "def", d, "next")["declarations"].([]any); len(ds) != 1 {
		t.Errorf("def next = %v", ds)
	}
	agentJSON(t, 1, "def", d, "nosuchname")

	refs := agentJSON(t, 0, "refs", d, "Counter.next")["references"].([]any)
	decl := 0
	for _, r := range refs {
		if r.(map[string]any)["declaration"] == true {
			decl++
		}
	}
	// the declaration and the two calls in twice
	if len(refs) != 3 || decl != 1 {
		t.Errorf("refs Counter.next = %v", refs)
	}

	ty := agentJSON(t, 0, "type", file+":30:7")
	if !strings.Contains(ty["signature"].(string), "Counter") {
		t.Errorf("type = %v", ty)
	}
	var out, errOut bytes.Buffer
	if got := run(t, []string{"type"}, &out, &errOut); got != 2 {
		t.Errorf("type with no position: exit %d, want 2: %s%s", got, out.String(), errOut.String())
	}

	doc := agentJSON(t, 0, "doc", d, "Counter")["declarations"].([]any)
	if len(doc) != 1 || !strings.Contains(doc[0].(map[string]any)["doc"].(string), "A counter") || len(doc[0].(map[string]any)["methods"].([]any)) != 1 {
		t.Errorf("doc Counter = %v", doc)
	}

	// the directory's own declarations, private ones too (decision 0289);
	// --pub for the exported surface
	syms := agentJSON(t, 0, "symbols", dir)["symbols"].([]any)
	names := map[string]bool{}
	for _, s := range syms {
		names[s.(map[string]any)["name"].(string)] = true
	}
	if !names["Counter"] || !names["Counter.next"] || !names["load"] || !names["read_it"] {
		t.Errorf("symbols = %v", names)
	}
	pub := agentJSON(t, 0, "symbols", "--pub", dir)["symbols"].([]any)
	for _, s := range pub {
		if s.(map[string]any)["name"] == "read_it" {
			t.Errorf("symbols --pub lists read_it: %v", pub)
		}
	}
	if len(pub) >= len(syms) {
		t.Errorf("symbols --pub (%d) not fewer than all (%d)", len(pub), len(syms))
	}
}

// TestAgentRootHints covers the two silent answers of the 2026-10-20
// poll: symbols of a root with no .xo files of its own names the
// directories below it, and a directory with no .xo files at all says so
// instead of "no declaration".
func TestAgentRootHints(t *testing.T) {
	t.Parallel()
	root := writeFiles(t, map[string]string{"app/main.xo": toolsProg})
	e := agentJSON(t, 1, "symbols", root)["error"].(string)
	if !strings.Contains(e, "no .xo files directly in") || !strings.Contains(e, "--dir=app") {
		t.Errorf("symbols at a root: %s", e)
	}
	if syms := agentJSON(t, 0, "symbols", filepath.Join(root, "app"))["symbols"].([]any); len(syms) == 0 {
		t.Errorf("symbols of app: none")
	}
	empty := t.TempDir()
	if e := agentJSON(t, 1, "def", "--dir="+empty, "load")["error"].(string); !strings.Contains(e, "no .xo files in or under") {
		t.Errorf("def in an empty directory: %s", e)
	}
}

// TestAgentNames covers the name forms of the 2026-10-20 tool polls
// (decision 0289): every tool resolves a name as code writes it, std and
// runtime modules the program does not import included.
func TestAgentNames(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"main.xo": toolsProg,
		"use.xo":  "use std/json as j\n\npub fn show_it(n: Int) -> Str {\n  j.encode_str(n)\n}\n",
	})
	d := "--dir=" + dir
	firstFile := func(args ...string) string {
		t.Helper()
		ds := agentJSON(t, 0, args...)["declarations"].([]any)
		if len(ds) == 0 {
			t.Fatalf("%v: no declarations", args)
		}
		return ds[0].(map[string]any)["file"].(string)
	}
	// `xo def Str.find` failed while `xo query def Str.find` worked
	if f := firstFile("def", d, "Str.find"); !strings.HasSuffix(f, "prelude.xo") {
		t.Errorf("def Str.find in %s", f)
	}
	// a type's member, not the runtime module chan.xo (which a
	// case-insensitive file system finds for Chan.xo)
	if f := firstFile("def", d, "Chan.new"); !strings.HasSuffix(f, "prelude.xo") {
		t.Errorf("def Chan.new in %s", f)
	}
	// std and runtime modules the program does not import
	if f := firstFile("def", d, "tls.client"); !strings.HasSuffix(f, "std/tls.xo") {
		t.Errorf("def tls.client in %s", f)
	}
	if f := firstFile("def", d, "pool.park_wait"); !strings.HasSuffix(f, "llrt/pool.xo") {
		t.Errorf("def pool.park_wait in %s", f)
	}
	// the native backends' std modules (std/native/X.xo), by their name
	// alone and by their path (2026-10-23: both answered "no declaration
	// named", with the declaration as the did-you-mean)
	for _, name := range []string{"prelude.str_trim_end_chars", "native/prelude.str_trim_end_chars"} {
		if f := firstFile("def", d, name); !strings.HasSuffix(f, "std/native/prelude.xo") {
			t.Errorf("def %s in %s", name, f)
		}
	}
	if f := firstFile("def", d, "http.handoff_response"); !strings.HasSuffix(f, "std/native/http.xo") {
		t.Errorf("def http.handoff_response in %s", f)
	}
	if f := firstFile("doc", d, "std/tls.client"); !strings.HasSuffix(f, "std/tls.xo") {
		t.Errorf("doc std/tls.client in %s", f)
	}
	// an import's local name
	if f := firstFile("def", d, "j.encode_str"); !strings.HasSuffix(f, "std/json.xo") {
		t.Errorf("def j.encode_str in %s", f)
	}
	// a near miss names where the function is
	miss := agentJSON(t, 1, "def", d, "nosuchmod.park_wait")["error"].(string)
	if !strings.Contains(miss, "pool.park_wait") {
		t.Errorf("def nosuchmod.park_wait: %s", miss)
	}
	// refs finds the runtime's cross-module call (tasks.xo's call of
	// pool.park_wait)
	refs := agentJSON(t, 0, "refs", d, "pool.park_wait")["references"].([]any)
	tasks := false
	for _, r := range refs {
		if strings.HasSuffix(r.(map[string]any)["file"].(string), "llrt/tasks.xo") {
			tasks = true
		}
	}
	if !tasks {
		t.Errorf("refs pool.park_wait has no call in tasks.xo: %v", refs)
	}

	// xo query: the same resolver, flags before the kind, the callers of
	// the right park_wait
	var out, errOut bytes.Buffer
	if got := run(t, []string{"query", "--json", d, "callers", "pool.park_wait"}, &out, &errOut); got != 0 {
		t.Fatalf("query callers pool.park_wait: exit %d: %s%s", got, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "park_wait_n") || !strings.Contains(out.String(), "scope_join") {
		t.Errorf("query callers pool.park_wait = %s", out.String())
	}
	out.Reset()
	errOut.Reset()
	if got := run(t, []string{"query", "--text", "effects", d, "main.load"}, &out, &errOut); got != 0 || !strings.Contains(out.String(), "fs.read") {
		t.Errorf("query --text effects main.load: exit %d: %s%s", got, out.String(), errOut.String())
	}
	// query type finds the file's module without --dir
	out.Reset()
	errOut.Reset()
	if got := run(t, []string{"query", "type", "--json", filepath.Join(dir, "main.xo") + ":30:7"}, &out, &errOut); got != 0 || !strings.Contains(out.String(), "Counter") {
		t.Errorf("query type without --dir: exit %d: %s%s", got, out.String(), errOut.String())
	}

	// an interface method has a signature too (Lib 1, 2026-10-20)
	out.Reset()
	errOut.Reset()
	if got := run(t, []string{"query", "def", "--json", d, "Fs.read"}, &out, &errOut); got != 0 || !strings.Contains(out.String(), `"fn Fs.read(`) {
		t.Errorf("query def Fs.read: exit %d: %s%s", got, out.String(), errOut.String())
	}

	// --text: the declaration as text, not JSON
	out.Reset()
	errOut.Reset()
	if got := run(t, []string{"def", "--text", d, "Counter.next"}, &out, &errOut); got != 0 || !strings.Contains(out.String(), "func Counter.next") || strings.HasPrefix(out.String(), "{") {
		t.Errorf("def --text Counter.next: exit %d: %s%s", got, out.String(), errOut.String())
	}
}

// TestVetStd covers xo vet of std and runtime modules (decision 0289):
// checked as std, with no check errors, by --std-module or a std
// directory; and xo vet of one file.
func TestVetStd(t *testing.T) {
	t.Parallel()
	vet := func(args ...string) map[string]any {
		t.Helper()
		var out, errOut bytes.Buffer
		if got := run(t, append([]string{"vet", "--json"}, args...), &out, &errOut); got != 0 {
			t.Fatalf("vet %v: exit %d: %s%s", args, got, out.String(), errOut.String())
		}
		var v map[string]any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("vet %v: not JSON: %v\n%s", args, err, out.String())
		}
		return v
	}
	for _, args := range [][]string{{"--std-module=native/llrt/pool"}, {"--std-module=json"}, {filepath.Join(xoStd(), "native", "llrt")}} {
		v := vet(args...)
		if fmt.Sprint(v["check_errors"]) != "0" {
			t.Errorf("vet %v: check_errors %v", args, v["check_errors"])
		}
	}
	dir := writeFiles(t, map[string]string{"main.xo": toolsProg, "other.xo": "pub fn other() {\n  dbg(1)\n}\n"})
	// DIR/... still means DIR (decision 0210)
	vet(dir + "/...")
	v := vet(filepath.Join(dir, "main.xo"))
	ds, _ := v["diagnostics"].([]any)
	if len(ds) == 0 {
		t.Errorf("vet main.xo: no findings (want its dbg): %v", v)
	}
	for _, d := range ds {
		if f, _ := d.(map[string]any)["span"].(map[string]any)["file"].(string); !strings.HasSuffix(f, "main.xo") {
			t.Errorf("vet main.xo reports %s", f)
		}
	}
}

// TestVetDeepTypes: xo vet counts the heap fields of each type once. The
// count walked the types afresh at every level, and on enums that nest
// each other in every variant (the compiler's own syntax trees) `xo vet
// compiler` ran for 25 minutes (2026-10-18); a recursive type counts
// itself once.
func TestVetDeepTypes(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	// eight enums of eight variants, each holding three of the others
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, "pub enum E%d {\n", i)
		for v := 0; v < 8; v++ {
			fmt.Fprintf(&b, "  V%d(a: E%d?, b: E%d?, c: List[E%d], d: Str)\n", v, (i+1)%8, (i+2)%8, (i+3)%8)
		}
		b.WriteString("}\n\n")
	}
	b.WriteString("pub struct Holder {\n  e: E0?\n}\n\npub fn first(h: Holder) -> Int {\n  match h.e {\n    Some(x) => 1\n    None => 0\n  }\n}\n")
	dir := writeFiles(t, map[string]string{"main.xo": b.String()})
	done := make(chan int, 1)
	var out, errOut bytes.Buffer
	go func() { done <- run(t, []string{"vet", "--json", dir}, &out, &errOut) }()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("xo vet: exit %d\n%s%s", code, out.String(), errOut.String())
		}
	case <-time.After(60 * time.Second):
		t.Fatal("xo vet did not finish in 60 s on nested enums")
	}
}

// TestVetTaskCapture covers XO0534 (decision 0294): a spawned task that
// captures a struct of 33 heap fields gets a note at the capture (line
// 44), while capturing one field bound before the spawn, a small struct,
// or the big struct's own parameter inside a plain closure does not.
func TestVetTaskCapture(t *testing.T) {
	t.Parallel()
	var fields, inits []string
	for i := 0; i < 33; i++ {
		fields = append(fields, fmt.Sprintf("  f%d: List[Int]", i))
		inits = append(inits, fmt.Sprintf("f%d: [%d]", i, i))
	}
	src := "struct Big {\n" + strings.Join(fields, "\n") + "\n}\n\n" +
		`struct Small {
  a: List[Int]
}

fn run(big: Big, small: Small) -> Int {
  let f0 = big.f0
  let n = scope s {
    let a = s.spawn(fn() { big.f1.len() })
    let b = s.spawn(fn() { f0.len() + small.a.len() })
    let g = fn() -> Int { big.f2.len() }
    a.await + b.await + g()
  }
  n
}

fn main(os: Os) {
  os.stdio.println("${run(Big{` + strings.Join(inits, ", ") + `}, Small{a: [1]})}")
}
`
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Span     struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if res.CheckErrors != 0 {
		t.Fatalf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	var got []string
	for _, d := range res.Diagnostics {
		if d.Code == "XO0534" {
			got = append(got, fmt.Sprintf("%s %d %s", d.Code, d.Span.Line, d.Severity))
			if !strings.Contains(d.Message, "`big`") || !strings.Contains(d.Message, "33 heap fields of `Big`") {
				t.Errorf("message: %s", d.Message)
			}
		}
	}
	want := "XO0534 44 note"
	if strings.Join(got, ", ") != want {
		t.Errorf("got %v, want %s\n%s", got, want, out.String())
	}
}

// TestVetIODefault covers XO0903: an Fs result's error turned into a
// default by `.ok() ?? d` (line 3) or `Err(_) => <empty>` (line 17)
// warns; an allow comment with a reason (line 10) silences it, one
// without a reason (line 32) does not; a match that names NotFound and a
// non-I/O `??` do not warn.
func TestVetIODefault(t *testing.T) {
	t.Parallel()
	src := `fn files(os: Os, d: Str) -> List[Str] uses fs.read {
  var out: List[Str] = []
  for e in os.fs.read_dir(d).ok() ?? [] {
    out.push(e.name)
  }
  out
}

fn hint(os: Os, p: Str) -> Str uses fs.read {
  // xo:allow XO0903 a hint: a failed read rebuilds
  (os.fs.read_text(p).ok() ?? "").trim()
}

fn src(os: Os, p: Str) -> Bytes uses fs.read {
  match os.fs.read(p) {
    Ok(b) => b
    Err(_) => b""
  }
}

fn src2(os: Os, p: Str) -> Bytes uses fs.read {
  match os.fs.read(p) {
    Ok(b) => b
    Err(NotFound(_)) => b""
    Err(_) => b""
  }
}

fn n(xs: List[Int]) -> Int { xs.first() ?? 0 }

fn bare(os: Os, p: Str) -> Str uses fs.read {
  // xo:allow XO0903
  os.fs.read_text(p).ok() ?? ""
}

fn main(os: Os) {
  os.stdio.println("${files(os, ".").len()} ${hint(os, "x")} ${src(os, "x").len()} ${src2(os, "x").len()} ${n([1])} ${bare(os, "x")}")
}
`
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Span     struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if res.CheckErrors != 0 {
		t.Fatalf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	var got []string
	for _, d := range res.Diagnostics {
		if d.Code == "XO0903" {
			got = append(got, fmt.Sprintf("%d %s", d.Span.Line, d.Severity))
			if !strings.Contains(d.Message, "FsErr") {
				t.Errorf("message: %s", d.Message)
			}
		}
	}
	if want := "3 warning, 17 warning, 33 warning"; strings.Join(got, ", ") != want {
		t.Errorf("XO0903 at %v, want %s\n%s", got, want, out.String())
	}
}

// TestVetMatchExit covers XO0906 (decision 0321): the four line `match`
// that only unwraps an Option (line 2) or a Result (lines 12 and 18) or
// leaves gets a note whose fix writes it with `??`; a Result arm that
// does not leave (line 24) and a three arm match (line 28) do not.
func TestVetMatchExit(t *testing.T) {
	t.Parallel()
	src := `fn first(xs: List[Int]) -> Int {
  let x = match xs.first() {
    Some(v) => v
    None => return 0
  }
  x
}

fn total(ss: List[Str]) -> Int ! Str {
  var sum = 0
  for s in ss {
    let n = match num(s) {
      Ok(v) => v
      Err(_) => continue
    }
    sum += n
  }
  let m = match num("7") {
    Ok(v) => v
    Err(e) => {
      return Err("bad ${e:?}")
    }
  }
  let d = match num("8") {
    Ok(v) => v
    Err(_) => 0
  }
  let k = match num("9") {
    Ok(v) => v
    Err(_) if sum > 0 => return Err("guarded")
    Err(_) => return Err("unguarded")
  }
  Ok(sum + m + d + k)
}

fn main(os: Os) {
  os.stdio.println("${first([1])} ${total(["1"]) ?? _ => 0}")
}

fn num(s: Str) -> Int ! Str {
  if s.is_empty() {
    return Err("empty")
  }
  s.len()
}

// already written with ?? (field report 0.26.0, b07: vet offered to
// rewrite it into the same text)
fn already(ss: List[Str]) -> Int {
  var t = 0
  for s in ss {
    let n = num(s) ?? _ => continue
    t += n
  }
  t
}
`
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
			Fixes []struct {
				Edits []struct {
					Replace string `json:"replace"`
				} `json:"edits"`
			} `json:"fixes"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if res.CheckErrors != 0 {
		t.Fatalf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	var got []string
	for _, d := range res.Diagnostics {
		if d.Code != "XO0906" {
			continue
		}
		if len(d.Fixes) != 1 || len(d.Fixes[0].Edits) != 1 {
			t.Fatalf("fixes: %+v", d.Fixes)
		}
		got = append(got, fmt.Sprintf("%d %s", d.Span.Line, d.Fixes[0].Edits[0].Replace))
	}
	want := []string{
		"2 xs.first() ?? return 0",
		"12 num(s) ?? _ => continue",
		`18 num("7") ?? e => return Err("bad ${e:?}")`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("XO0906:\n%s\nwant:\n%s\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"), out.String())
	}
}

// TestVetDebugInterp covers XO0535 (decision 0296): `${xs.debug()}` gets
// a note whose fix writes `:?` (line 8), while `${xs:?}` itself and a
// type's own debug method do not.
func TestVetDebugInterp(t *testing.T) {
	t.Parallel()
	src := `struct P {
  n: Int
}

fn (p: P) debug() -> Str { "P" }

fn show(xs: List[Int], p: P) -> Str {
  "a ${xs.debug()} b ${xs:?} c ${p.debug()}"
}

fn main(os: Os) {
  os.stdio.println(show([1], P{n: 1}))
}
`
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
			Span     struct {
				Line int `json:"line"`
				Col  int `json:"col"`
			} `json:"span"`
			Fixes []struct {
				Edits []struct {
					Span struct {
						Line   int `json:"line"`
						Col    int `json:"col"`
						EndCol int `json:"end_col"`
					} `json:"span"`
					Replace string `json:"replace"`
				} `json:"edits"`
			} `json:"fixes"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if res.CheckErrors != 0 {
		t.Fatalf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	var got []string
	for _, d := range res.Diagnostics {
		if d.Code != "XO0535" {
			continue
		}
		got = append(got, fmt.Sprintf("%d %s", d.Span.Line, d.Severity))
		// `${xs.debug()}`: the edit replaces `.debug()` (columns 10 to 18
		// of line 8) with `:?`
		if len(d.Fixes) != 1 || len(d.Fixes[0].Edits) != 1 {
			t.Fatalf("fixes: %+v", d.Fixes)
		}
		e := d.Fixes[0].Edits[0]
		line := strings.Split(src, "\n")[e.Span.Line-1]
		fixed := line[:e.Span.Col-1] + e.Replace + line[e.Span.EndCol-1:]
		if fixed != `  "a ${xs:?} b ${xs:?} c ${p.debug()}"` {
			t.Errorf("fixed line %q", fixed)
		}
	}
	if strings.Join(got, ", ") != "8 note" {
		t.Errorf("XO0535 at %v, want [8 note]\n%s", got, out.String())
	}
}

// TestVetElemCopyStore covers XO0536 (decision 0294's addendum): a list
// element copied out, changed, and stored back is noted (line 3); taking
// it first, changing it in place, and a struct element are not.
func TestVetElemCopyStore(t *testing.T) {
	t.Parallel()
	src := `fn fill(var bufs: List[List[Int]], k: Int, n: Int) {
  for i in 0..n {
    var b = bufs[k]
    b.push(i)
    bufs[k] = b
  }
}

fn take(var bufs: List[List[Int]], k: Int) {
  var b = bufs[k]
  bufs[k] = []
  b.push(1)
  bufs[k] = b
}

fn inplace(var bufs: List[List[Int]], k: Int) {
  bufs[k].push(1)
}

struct P {
  n: Int
}

fn structs(var ps: List[P], k: Int) {
  var p = ps[k]
  p.n += 1
  ps[k] = p
}

fn main(os: Os) {
  var bufs: List[List[Int]] = [[], []]
  fill(bufs, 0, 3)
  take(bufs, 1)
  inplace(bufs, 1)
  var ps = [P{n: 1}]
  structs(ps, 0)
  os.stdio.println("${bufs.len()} ${ps.len()}")
}
`
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Span     struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if res.CheckErrors != 0 {
		t.Fatalf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	var got []string
	for _, d := range res.Diagnostics {
		if d.Code == "XO0536" {
			got = append(got, fmt.Sprintf("%d %s", d.Span.Line, d.Severity))
			if !strings.Contains(d.Message, "`b` is copied out of `bufs[k]`") {
				t.Errorf("message: %s", d.Message)
			}
		}
	}
	if strings.Join(got, ", ") != "3 note" {
		t.Errorf("XO0536 at %v, want [3 note]\n%s", got, out.String())
	}
}

// TestVetEmbedUntracked covers XO0904 (decision 0304): of three embedded
// files, the one git ignores and the one git does not track get the
// warning at their constants (lines 3 and 4); the tracked one does not.
func TestVetEmbedUntracked(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":        "module example.com/e\n",
		".gitignore":    "secret.txt\n",
		"tracked.txt":   "a\n",
		"secret.txt":    "b\n",
		"untracked.txt": "c\n",
		"main.xo": `const A: Str = embed("tracked.txt")

const B: Str = embed("secret.txt")
const C: Str = embed("untracked.txt")

fn main(os: Os) {
  os.stdio.println(A + B + C)
}
`,
	})
	for _, args := range [][]string{{"init", "-q"}, {"add", "xo.mod", ".gitignore", "tracked.txt", "main.xo"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Span    struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var got []string
	for _, d := range res.Diagnostics {
		if d.Code == "XO0904" {
			got = append(got, fmt.Sprintf("%d %t", d.Span.Line, strings.Contains(d.Message, "ignores")))
		}
	}
	if want := "3 true, 4 false"; strings.Join(got, ", ") != want {
		t.Errorf("XO0904 at %v, want %s\n%s", got, want, out.String())
	}
}

// TestAgentRuntimeRefs: runtime (std/native/llrt) names by their file's
// name (Xo Lang 2, 2026-10-21: `pool.park_wait` gave "no declaration"
// and refs found no callers). refs lists the calls inside the runtime
// (tasks.xo's qualified call and pool.xo's own), once each, from a
// program's directory, from std/native, and from the runtime directory,
// where each file is its own module (a bare name lists every file's).
func TestAgentRuntimeRefs(t *testing.T) {
	t.Parallel()
	prog := writeFiles(t, map[string]string{"main.xo": toolsProg})
	std := xoStd()
	refFiles := func(dir string) []string {
		t.Helper()
		v := agentJSON(t, 0, "refs", "--dir="+dir, "--json", "pool.park_wait")
		var files []string
		seen := map[string]bool{}
		for _, r := range v["references"].([]any) {
			m := r.(map[string]any)
			k := fmt.Sprintf("%s:%v:%v", m["file"], m["line"], m["col"])
			if seen[k] {
				t.Errorf("refs from %s lists %s twice", dir, k)
			}
			seen[k] = true
			files = append(files, filepath.Base(m["file"].(string)))
		}
		return files
	}
	for _, dir := range []string{prog, filepath.Join(std, "native"), filepath.Join(std, "native", "llrt")} {
		fs := refFiles(dir)
		pools, tasks := 0, 0
		for _, f := range fs {
			switch f {
			case "pool.xo":
				pools++
			case "tasks.xo":
				tasks++
			}
		}
		// the declaration and the call in pool.xo, the call in tasks.xo
		if pools < 2 || tasks < 1 {
			t.Errorf("refs pool.park_wait from %s: %v", dir, fs)
		}
	}
	ds := agentJSON(t, 0, "def", "--dir="+filepath.Join(std, "native", "llrt"), "--json", "park_wait")["declarations"].([]any)
	var where []string
	for _, d := range ds {
		where = append(where, filepath.Base(d.(map[string]any)["file"].(string)))
	}
	if len(where) < 3 {
		t.Errorf("def park_wait in the runtime directory: %v, want pool.xo, poll.xo, and tasks.xo", where)
	}
}

// TestAgentRefsTestdata: refs at a root leaves out its testdata
// directories, as every DIR/... walk does, and names them
// ("testdata_skipped", a note in text); --testdata finds the calls in
// the modules there (the program goldens: DS-2, 2026-10-25, `xo refs
// prelude.Str.contains_where --dir=.` missed golden/strwhere), leaving
// out a module whose requirement is not in the module cache, which
// stopped the whole check. From a program's directory, refs of a std
// name lists its uses in std and in the runtime: std modules are checked
// only when their text spells the name, and the runtime's pgwire.xo,
// which a filtered out native module used to bring in, stays.
func TestAgentRefsTestdata(t *testing.T) {
	t.Parallel()
	prog := "fn main(os: Os) {\n  os.stdio.println(\"${\"a1\".contains_where(fn(r) { r == '1' })}\")\n}\n"
	root := writeFiles(t, map[string]string{
		"app/xo.mod":                     "module example.com/app\n",
		"app/main.xo":                    prog,
		"app/testdata/golden/w/xo.mod":   "module example.com/w\n",
		"app/testdata/golden/w/main.xo":  prog,
		"app/testdata/golden/net/xo.mod": "module example.com/net\n\nrequire example.com/xo-refs-never-fetched v1.0.0\n",
		// contains_where in a comment: refs checks it (it spells the name)
		"app/testdata/golden/net/main.xo": "use example.com/xo-refs-never-fetched/lib\n\n// not contains_where\nfn main(os: Os) {\n  os.stdio.println(lib.name())\n}\n",
	})
	userRefs := func(v map[string]any) []string {
		var files []string
		for _, r := range v["references"].([]any) {
			f := r.(map[string]any)["file"].(string)
			if strings.HasPrefix(f, root) || !strings.Contains(f, "std/") {
				files = append(files, filepath.ToSlash(f))
			}
		}
		return files
	}
	v := agentJSON(t, 0, "refs", "--dir="+root, "--json", "prelude.Str.contains_where")
	if got := userRefs(v); len(got) != 1 || !strings.HasSuffix(got[0], "app/main.xo") {
		t.Errorf("refs at the root: %v, want app/main.xo's call alone", got)
	}
	if td, _ := v["testdata_skipped"].([]any); len(td) != 1 || td[0] != "app/testdata" {
		t.Errorf("testdata_skipped = %v, want [app/testdata]", v["testdata_skipped"])
	}
	v = agentJSON(t, 0, "refs", "--dir="+root, "--json", "--testdata", "prelude.Str.contains_where")
	got := userRefs(v)
	if joined := strings.Join(got, " "); len(got) != 2 || !strings.Contains(joined, "app/testdata/golden/w/main.xo") || !strings.Contains(joined, "app/main.xo") {
		t.Errorf("refs --testdata: %v, want app/main.xo's and golden/w's calls", got)
	}
	if sk, _ := v["skipped"].([]any); len(sk) != 1 || !strings.HasSuffix(sk[0].(string), "golden/net") {
		t.Errorf("refs --testdata: skipped = %v, want golden/net (its requirement is not in the module cache)", v["skipped"])
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"refs", "--dir=" + root, "--text", "prelude.Str.contains_where"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "note: not checked: app/testdata; --testdata checks them too") {
		t.Errorf("refs --text at the root: exit %d: %s%s", code, out.String(), errOut.String())
	}

	// a std name from a program's directory: the runtime's uses too
	pgwire := false
	for _, r := range agentJSON(t, 0, "refs", "--dir="+filepath.Join(root, "app"), "--json", "prelude.Str.trim_prefix")["references"].([]any) {
		if strings.HasSuffix(filepath.ToSlash(r.(map[string]any)["file"].(string)), "native/llrt/pgwire.xo") {
			pgwire = true
		}
	}
	if !pgwire {
		t.Errorf("refs prelude.Str.trim_prefix from a program lists no call in std/native/llrt/pgwire.xo")
	}
}

// TestAgentRefsBatch: `xo refs A B C` answers every name from one check
// (DS-2, 2026-10-25: a sweep of N names paid the std load N times), in
// order, a name with no answer an {"target", "error"} entry and exit 1;
// the text form prints each answer, a blank line between.
func TestAgentRefsBatch(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": "fn one() -> Int { 1 }\n\nfn two() -> Int { one() + 1 }\n\nfn main(os: Os) {\n  os.stdio.println(\"${two()}\".trim_prefix(\"x\"))\n}\n"})
	v := agentJSON(t, 1, "refs", "--dir="+dir, "--json", "one", "prelude.Str.trim_prefix", "no_such_name")
	rs := v["results"].([]any)
	if len(rs) != 3 {
		t.Fatalf("results: %v", v)
	}
	if n := len(rs[0].(map[string]any)["references"].([]any)); n != 2 {
		t.Errorf("one: %d references, want the declaration and the call", n)
	}
	pgwire := false
	for _, r := range rs[1].(map[string]any)["references"].([]any) {
		if strings.HasSuffix(filepath.ToSlash(r.(map[string]any)["file"].(string)), "native/llrt/pgwire.xo") {
			pgwire = true
		}
	}
	if !pgwire {
		t.Errorf("prelude.Str.trim_prefix in a batch lists no call in pgwire.xo: %v", rs[1])
	}
	if e, _ := rs[2].(map[string]any)["error"].(string); rs[2].(map[string]any)["target"] != "no_such_name" || e == "" {
		t.Errorf("no_such_name: %v, want its error", rs[2])
	}
	var out, errOut bytes.Buffer
	code := run(t, []string{"refs", "--dir=" + dir, "--text", "one", "two"}, &out, &errOut)
	if code != 0 || strings.Count(out.String(), "reference(s)") != 2 || !strings.Contains(out.String(), "2 reference(s)\n\n") {
		t.Errorf("refs --text one two: exit %d:\n%s%s", code, out.String(), errOut.String())
	}
	if code := run(t, []string{"refs", "--dir=" + dir, "one", "main.xo:1:4"}, &out, &errOut); code != 2 {
		t.Errorf("refs with a position among several names: exit %d, want 2", code)
	}
}

// runEnv is run with env added to the xo process's environment.
func runEnv(t testing.TB, env, args []string, stdout, stderr io.Writer) int {
	cli, err := xoCommand()
	if err != nil {
		fmt.Fprintf(stderr, "no xo to run: %v\n", err)
		return -1
	}
	cmd := command(t, cli[0], append(append([]string{}, cli[1:]...), args...)...)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return -1
	}
	return 0
}

// TestAgentRefsSpellFilter: refs at a root checks only the modules whose
// text spells the name's last segment as a whole identifier (2026-10-26:
// --testdata at the xo root took 5 to 12 s and up to 6 GB checking every
// module), and leaves unchecked a function body that spells none of
// them (Checker.body_words: trim_prefix at the xo root with --testdata,
// 5.8 GB to 1.0 GB), and finds the same references as a check of every
// module and body (XO_REFS_SPELL=off) for a function of the program, a
// std function and a prelude method, with and without --testdata; app's
// quiet spells none, wrapped names lib.helper only inside a closure.
func TestAgentRefsSpellFilter(t *testing.T) {
	t.Parallel()
	root := writeFiles(t, map[string]string{
		"app/xo.mod":             "module example.com/app\n",
		"app/main.xo":            "use example.com/lib\nuse std/json\n\nfn quiet(n: Int) -> Int { n * 3 }\n\nfn wrapped() -> Int {\n  let f = fn(x: Int) -> Int { lib.helper(x) }\n  f(quiet(2))\n}\n\nfn main(os: Os) {\n  let n = lib.helper(wrapped())\n  let v = json.decode[Int](\"2\".bytes()) ?? e => 0\n  os.stdio.println(\"${n} ${v} ${\"ab\".trim_prefix(\"a\")}\")\n}\n",
		"lib/xo.mod":             "module example.com/lib\n",
		"lib/lib.xo":             "pub fn helper(n: Int) -> Int { n + 1 }\n\npub fn helper_two(n: Int) -> Int { helper(n) * 2 }\n",
		"other/xo.mod":           "module example.com/other\n",
		"other/main.xo":          "fn main(os: Os) {\n  os.stdio.println(\"helpers decode trim\")\n}\n",
		"app/testdata/g/xo.mod":  "module example.com/g\n",
		"app/testdata/g/main.xo": "use std/json\n\nfn main(os: Os) {\n  os.stdio.println(\"${\"xy\".trim_prefix(\"x\")}\")\n  let _r = json.decode[Int](\"1\".bytes()).ok()\n}\n",
	})
	refs := func(env string, args ...string) []string {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := runEnv(t, []string{env}, append([]string{"refs", "--dir=" + root, "--json"}, args...), &out, &errOut); code != 0 {
			t.Fatalf("refs %v (%s): exit %d: %s%s", args, env, code, out.String(), errOut.String())
		}
		var v struct {
			References []struct {
				File string `json:"file"`
				Line int    `json:"line"`
				Col  int    `json:"col"`
			} `json:"references"`
		}
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("refs %v: %v\n%s", args, err, out.String())
		}
		var got []string
		for _, r := range v.References {
			got = append(got, fmt.Sprintf("%s:%d:%d", r.File, r.Line, r.Col))
		}
		sort.Strings(got)
		return got
	}
	for _, name := range []string{"lib.helper", "json.decode", "prelude.Str.trim_prefix"} {
		for _, td := range [][]string{nil, {"--testdata"}} {
			args := append(append([]string{}, td...), name)
			on := refs("XO_REFS_SPELL=on", args...)
			off := refs("XO_REFS_SPELL=off", args...)
			if strings.Join(on, "\n") != strings.Join(off, "\n") || len(on) == 0 {
				t.Errorf("refs %v: with the filter %v, every module %v", args, on, off)
			}
		}
	}
}

// TestAgentRefsBrokenBody: a function body that spells the name looked
// for but does not parse (bench/paper's blind solutions) stops neither
// xo refs nor its other modules' answers; before, the lazy parse of the
// body faulted ("a lazily parsed body ... does not parse").
func TestAgentRefsBrokenBody(t *testing.T) {
	t.Parallel()
	root := writeFiles(t, map[string]string{
		"app/xo.mod":  "module example.com/app\n",
		"app/main.xo": "fn helper(n: Int) -> Int { n + 1 }\n\nfn main(os: Os) {\n  os.stdio.println(\"${helper(2)}\")\n}\n",
		"bad/xo.mod":  "module example.com/bad\n",
		"bad/main.xo": "fn broken() -> Int {\n  let x = helper(1\n  x +\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"${broken()}\")\n}\n",
	})
	var out, errOut bytes.Buffer
	code := runEnv(t, nil, []string{"refs", "--dir=" + root, "--json", "helper"}, &out, &errOut)
	if strings.Contains(out.String()+errOut.String(), "does not parse") {
		t.Fatalf("refs helper: the broken body stopped the lookup (exit %d): %s%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "app/main.xo") {
		t.Errorf("refs helper (exit %d): no reference in app/main.xo: %s%s", code, out.String(), errOut.String())
	}
}

// TestAgentLookupsEager: def, doc and symbols by name, and xo query's
// def, api, imports, deps and callers, parse and check no function body
// (callers only those that spell the name; 2026-10-26), and answer
// exactly as a check of every body (XO_LOOKUP_EAGER=1) does, in a module
// and at a root holding two.
func TestAgentLookupsEager(t *testing.T) {
	t.Parallel()
	root := writeFiles(t, map[string]string{
		"app/xo.mod":     "module example.com/app\n",
		"app/main.xo":    "use example.com/app/lib\nuse std/json\n\n/// Limit caps a count.\nconst LIMIT = 4\n\nfn quiet(n: Int) -> Int {\n  let inner = fn(x: Int) -> Int { x * 3 }\n  inner(n)\n}\n\nfn main(os: Os) {\n  var b = lib.Box.new(quiet(LIMIT))\n  b.grow(1)\n  let v = json.decode[Int](\"2\".bytes()) ?? e => 0\n  os.stdio.println(\"${lib.helper(b.n)} ${v} ${\"ab\".trim_prefix(\"a\")}\")\n}\n",
		"app/lib/lib.xo": "/// Box holds a count.\npub struct Box {\n  pub n: Int\n}\n\n/// new makes a Box.\npub fn Box.new(n: Int) -> Box { Box{n: n} }\n\n/// grow adds d.\npub fn (var b: Box) grow(d: Int) { b.n += helper(d) }\n\n/// helper adds one.\npub fn helper(n: Int) -> Int { n + 1 }\n",
		"other/xo.mod":   "module example.com/other\n",
		"other/main.xo":  "fn helper() -> Int {\n  let h = fn() -> Int { 2 }\n  h()\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"${helper()}\")\n}\n",
	})
	app := filepath.Join(root, "app")
	cases := []struct {
		dir  string
		args []string
	}{
		{app, []string{"def", "quiet"}},
		{app, []string{"def", "LIMIT"}},
		{app, []string{"def", "lib.helper"}},
		{app, []string{"def", "lib.Box.grow"}},
		{app, []string{"def", "Str.trim_prefix"}},
		{app, []string{"def", "json.decode"}},
		{app, []string{"doc", "lib.Box"}},
		{app, []string{"doc", "Str"}},
		{app, []string{"symbols"}},
		{app, []string{"symbols", "--module=example.com/app/lib"}},
		{root, []string{"def", "helper"}},
		{root, []string{"def", "Box.grow"}},
		{root, []string{"doc", "Box"}},
		{root, []string{"symbols", "--module=example.com/app/lib"}},
		{root, []string{"refs", "lib.helper"}},
		{app, []string{"refs", "lib.helper"}},
		{app, []string{"query", "def", "lib.Box.grow"}},
		{app, []string{"query", "api", "example.com/app/lib"}},
		{app, []string{"query", "imports", "example.com/app"}},
		{app, []string{"query", "deps", "example.com/app"}},
		{app, []string{"query", "callers", "lib.helper"}},
		{root, []string{"query", "def", "helper"}},
		{root, []string{"query", "callers", "example.com/app/lib.helper"}},
	}
	for _, c := range cases {
		args := append(append([]string{}, c.args...), "--dir="+c.dir, "--json")
		var lazy, lazyErr, eager, eagerErr bytes.Buffer
		lc := runEnv(t, nil, args, &lazy, &lazyErr)
		ec := runEnv(t, []string{"XO_LOOKUP_EAGER=1"}, args, &eager, &eagerErr)
		if lc != 0 || lazy.Len() == 0 {
			t.Errorf("%v: exit %d: %s%s", args, lc, lazy.String(), lazyErr.String())
			continue
		}
		if lc != ec || lazy.String() != eager.String() {
			t.Errorf("%v: declarations only, exit %d:\n%s\nevery body, exit %d:\n%s%s", args, lc, lazy.String(), ec, eager.String(), eagerErr.String())
		}
	}
}

// TestAgentRefsSamePath: three modules with one module path under a
// root (the xo checkout's example.com/notes: bench/paper/v0.3 and
// bench/xo/v0.4's notes services and internal/check's imports_deny
// golden) are not one module: the first is checked and the others are
// named (JSON "same_path", a note), where a whole-root check read only
// the first module's files and lost the others' references without a
// word (2026-10-26: json.decode in bench/xo/v0.4's store.xo).
func TestAgentRefsSamePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, src := range map[string]string{
		"a-paper":  filepath.Join(repoRoot, "bench", "paper", "v0.3", "08-notes-service"),
		"b-xo":     filepath.Join(repoRoot, "bench", "xo", "v0.4", "08-notes-service"),
		"c-golden": filepath.Join(repoRoot, "internal", "check", "testdata", "golden", "imports_deny"),
	} {
		if err := os.CopyFS(filepath.Join(root, name), os.DirFS(src)); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	// every module checked, so all three meet
	code := runEnv(t, []string{"XO_REFS_SPELL=off"}, []string{"refs", "--dir=" + root, "--json", "prelude.Str.to_lower"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("refs: exit %d: %s%s", code, out.String(), errOut.String())
	}
	var v struct {
		References []struct {
			File string `json:"file"`
		} `json:"references"`
		SamePath []struct {
			Dir  string `json:"dir"`
			Path string `json:"path"`
			Kept string `json:"kept"`
		} `json:"same_path"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	var dirs []string
	for _, x := range v.SamePath {
		if x.Path != "example.com/notes" || x.Kept != "a-paper" {
			t.Errorf("same_path entry %+v, want example.com/notes kept as a-paper", x)
		}
		dirs = append(dirs, x.Dir)
	}
	sort.Strings(dirs)
	if strings.Join(dirs, " ") != "b-xo c-golden" {
		t.Errorf("same_path dirs %v, want b-xo and c-golden", dirs)
	}
	// the kept module's references are there; the others' are not
	// claimed
	paper := false
	for _, r := range v.References {
		if strings.Contains(r.File, "/b-xo/") || strings.Contains(r.File, "/c-golden/") {
			t.Errorf("a reference in a module left out: %s", r.File)
		}
		if strings.Contains(r.File, "/a-paper/") {
			paper = true
		}
	}
	if !paper {
		t.Errorf("no reference in the kept module a-paper: %+v", v.References)
	}
	out.Reset()
	errOut.Reset()
	if code := runEnv(t, []string{"XO_REFS_SPELL=off"}, []string{"refs", "--dir=" + root, "--text", "prelude.Str.to_lower"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "note: not checked, another module under the root has its module path") {
		t.Errorf("refs --text: exit %d, no note:\n%s%s", code, out.String(), errOut.String())
	}
	// one module alone (--dir) still answers for it
	out.Reset()
	if code := run(t, []string{"refs", "--dir=" + filepath.Join(root, "b-xo"), "--json", "prelude.Str.to_lower"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "/b-xo/") {
		t.Errorf("refs --dir=b-xo: exit %d:\n%s", code, out.String())
	}
}

// TestAgentRootSkipsBrokenModule: at a root whose modules include one
// whose `use c` binding is missing (the xo checkout's
// examples/realtime-control), the lookups answer for the other modules
// and name the skipped one ("skipped" in JSON, a note in text) instead of
// "no declaration" for every name.
func TestAgentRootSkipsBrokenModule(t *testing.T) {
	t.Parallel()
	root := writeFiles(t, map[string]string{
		"good/xo.mod":  "module example.com/good\n",
		"good/main.xo": "fn helper(n: Int) -> Int { n + 1 }\n\nfn main(os: Os) {\n  os.stdio.println(\"${helper(1)}\")\n}\n",
		"bad/xo.mod":   "module example.com/bad\n",
		"bad/clock.h":  "long now_ns(void);\n",
		// it spells helper (a comment), so refs checks it (only a module that
		// spells the name can refer to it) and finds it broken
		"bad/main.xo": "use c \"clock.h\" uses none\n\n// not the helper\nfn main(os: Os) {\n  os.stdio.println(\"${clock.now_ns()}\")\n}\n",
	})
	v := agentJSON(t, 0, "refs", "--dir="+root, "--json", "helper")
	refs := v["references"].([]any)
	if len(refs) != 2 {
		t.Errorf("refs helper: %d references, want the declaration and the call: %v", len(refs), refs)
	}
	sk, _ := v["skipped"].([]any)
	if len(sk) != 1 || !strings.HasSuffix(sk[0].(string), "bad") {
		t.Errorf("skipped = %v, want the bad module", v["skipped"])
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"def", "--dir=" + root, "--text", "helper"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "note: skipped") {
		t.Errorf("def --text: exit %d: %s%s", code, out.String(), errOut.String())
	}
}

// TestSymbolsScope: `xo symbols` of one file (--file, or a .xo argument)
// and of one module by import path (--module), std and runtime ones too;
// the directory's own declarations stay the default (agent feedback: the
// whole directory came back when one file was wanted).
func TestSymbolsScope(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"a.xo": "pub fn alpha() -> Int { 1 }\n\nfn main(os: Os) {\n  os.stdio.println(\"${alpha() + beta()}\")\n}\n",
		"b.xo": "fn beta() -> Int { 2 }\n\nstruct Bee {\n  n: Int\n}\n",
	})
	names := func(args ...string) []string {
		t.Helper()
		var out []string
		for _, s := range agentJSON(t, 0, append([]string{"symbols", "--json"}, args...)...)["symbols"].([]any) {
			out = append(out, s.(map[string]any)["name"].(string))
		}
		sort.Strings(out)
		return out
	}
	if got := strings.Join(names(dir), " "); got != "Bee Bee.n alpha beta main" {
		t.Errorf("the directory: %s", got)
	}
	if got := strings.Join(names("--file="+filepath.Join(dir, "b.xo")), " "); got != "Bee Bee.n beta" {
		t.Errorf("--file b.xo: %s", got)
	}
	if got := strings.Join(names(filepath.Join(dir, "a.xo")), " "); got != "alpha main" {
		t.Errorf("a.xo as the argument: %s", got)
	}
	if got := names("--dir="+dir, "--module=std/json", "--pub"); !slices.Contains(got, "encode_str") {
		t.Errorf("--module=std/json: %v", got)
	}
	if got := names("--dir="+dir, "--module=native/llrt/pool", "--pub"); !slices.Contains(got, "park_wait") {
		t.Errorf("--module=native/llrt/pool: %v", got)
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"symbols", "--file=x.xo", "--module=std/json"}, &out, &errOut); code != 2 {
		t.Errorf("--file with --module: exit %d, want 2: %s", code, errOut.String())
	}
}

// TestDevGoldensInlineStrict: `xo dev goldens` builds with
// XO_INLINE_STRICT=1 (an arm64 module whose inlining does not verify
// fails the build), as the Go tests do, unless the caller sets it: a gate
// of dev goldens passed http_server while the Go tests failed it on the
// verifier (2026-10-21). A fake xo records what it was given.
func TestDevGoldensInlineStrict(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake xo is a shell script")
	}
	dir := writeFiles(t, map[string]string{
		"golden/p/main.xo":  "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"golden/p/want.txt": "exit 0\n--- stdout\nhi\n--- stderr\n",
	})
	rec := filepath.Join(dir, "strict.txt")
	fake := filepath.Join(dir, "fakexo")
	// records XO_INLINE_STRICT and writes the -o output
	script := "#!/bin/sh\nprintf '%s\\n' \"${XO_INLINE_STRICT-unset}\" >> " + rec + "\nout=\nwhile [ $# -gt 0 ]; do if [ \"$1\" = -o ]; then out=$2; fi; shift; done\n[ -n \"$out\" ] && : > \"$out\"\nexit 0\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ env, want string }{{"", "1"}, {"0", "0"}} {
		os.Remove(rec)
		cli, err := xoCommand()
		if err != nil {
			t.Skip(err)
		}
		cmd := testtmp.Guard(exec.Command(cli[0], append(cli[1:], "dev", "goldens", "--xo="+fake, "--backend=arm64", "--build-only", "--no-each", "--force", "--no-cache", filepath.Join(dir, "golden"))...))
		env := []string{}
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "XO_INLINE_STRICT=") {
				env = append(env, kv)
			}
		}
		if c.env != "" {
			env = append(env, "XO_INLINE_STRICT="+c.env)
		}
		cmd.Env = env
		out, _ := cmd.CombinedOutput()
		b, err := os.ReadFile(rec)
		if err != nil {
			t.Fatalf("XO_INLINE_STRICT=%q: the fake xo never ran: %s", c.env, out)
		}
		if got := strings.TrimSpace(strings.Split(string(b), "\n")[0]); got != c.want {
			t.Errorf("XO_INLINE_STRICT=%q in the caller: the build got %q, want %q\n%s", c.env, got, c.want, out)
		}
	}
}

// TestGoCompiledMethodRouting: on the Go backend, a static, var, or
// generic method of a std module compiled from its Xo implementation
// (std/go_compiled.txt, decision 0312) is a build error naming it, not
// a silent call of std/rt's Go version (owner, 2026-10-21). A copy of std
// gives uuid a static method that a program calls.
func TestGoCompiledMethodRouting(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("copies std and builds")
	}
	std := filepath.Join(t.TempDir(), "std")
	if err := os.CopyFS(std, os.DirFS(filepath.Join("..", "..", "std"))); err != nil {
		t.Fatal(err)
	}
	gc, err := os.ReadFile(filepath.Join(std, "go_compiled.txt"))
	if err != nil || !strings.Contains(string(gc), "uuid") {
		t.Skip("uuid is not in std/go_compiled.txt")
	}
	f, err := os.OpenFile(filepath.Join(std, "uuid.xo"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("\n/// a test's static method\nfn (static: Uuid) xo_test_zero() -> Int\n")
	f.Close()
	dir := writeFiles(t, map[string]string{
		"main.xo": "use std/uuid\n\nfn main(os: Os) {\n  os.stdio.println(\"${uuid.Uuid.xo_test_zero()}\")\n}\n",
	})
	cli, err := xoCommand()
	if err != nil {
		t.Skip(err)
	}
	cmd := testtmp.Guard(exec.Command(cli[0], append(cli[1:], "build", "--backend=go", "--std="+std, "-o", filepath.Join(dir, "p"), dir)...))
	cmd.Env = append(os.Environ(), "XO_BIN_CACHE=off")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "uuid.Uuid.xo_test_zero`: a static method") {
		t.Errorf("want a build error naming uuid.Uuid.xo_test_zero, got (err %v):\n%s", err, out)
	}
}

// TestWasmToolRejectedText: the wasm tool (compiler/wasm) prints the
// checker's first error after `== rejected`, so a rejected run says why
// (owner, from the 2026-10-21 poll).
func TestWasmToolRejectedText(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds compiler/wasm")
	}
	cli, err := xoCommand()
	if err != nil {
		t.Skip(err)
	}
	tool := filepath.Join(t.TempDir(), "xowasm")
	b := testtmp.Guard(exec.Command(cli[0], append(cli[1:], "build", "-o", tool, filepath.Join("..", "..", "compiler", "wasm"))...))
	if out, err := b.CombinedOutput(); err != nil {
		t.Fatalf("building compiler/wasm: %v\n%s", err, out)
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/w\n",
		"main.xo": "fn main(os: Os) {\n  let n: Int = \"x\"\n  os.stdio.println(\"${n}\")\n}\n",
	})
	std, _ := filepath.Abs(filepath.Join("..", "..", "std"))
	// run in the directory, as the tool's callers do (an absolute
	// directory argument with --wd elsewhere reads the wrong directory:
	// gotchas/dev-tool-wd-is-not-cwd.md)
	run := testtmp.Guard(exec.Command(tool, "--std="+std, "."))
	run.Dir = dir
	out, _ := run.CombinedOutput()
	if !strings.Contains(string(out), "== rejected ") || !strings.Contains(string(out), ":2:") || !strings.Contains(string(out), "XO0401") {
		t.Errorf("want `== rejected FILE:2:COL: XO0401 ...`, got:\n%s", out)
	}
}

// TestVetRecvOrderRetired: XO0537 was retired with decision 0326, which
// defines `t.size(t.add(v))` (a by value receiver is read after the
// arguments); xo vet reports nothing for it.
func TestVetRecvOrderRetired(t *testing.T) {
	t.Parallel()
	src := `struct Table {
  items: List[Int]
}

fn (var t: Table) add(v: Int) -> Int {
  t.items.push(v)
  t.items.len() - 1
}

fn (t: Table) size(i: Int) -> Int { t.items[i] }

fn bad(var t: Table, v: Int) -> Int {
  t.size(t.add(v))
}

fn main(os: Os) {
  var t = Table{items: [1]}
  os.stdio.println("${bad(t, 2)}")
}
`
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "XO0537") {
		t.Errorf("xo vet still reports XO0537:\n%s", out.String())
	}
}

// vetCostProg has a loop over a log-mode Map (decision 0291: written
// twice while copies held it) whose body has a closure, so the loop
// copies every entry first (XO0907, line 23), and the same loop with a
// small body, which walks the table in place (no note).
const vetCostProg = `fn log_map(n: Int) -> (Map[Int, Str], Int) {
  var a: Map[Int, Str] = {}
  for i in 0..n {
    a[i] = "v${i}"
  }
  var b = a
  b[n] = "n"
  var c = b
  c[n + 1] = "n1"
  var keep = 0
  for (_, v) in a {
    keep += v.len()
  }
  for (_, v) in b {
    keep += v.len()
  }
  (c, keep)
}

fn copying(m: Map[Int, Str]) -> Int {
  var t = 0
  var fs: List[fn() -> Int] = []
  for (k, v) in m {
    fs.push(fn() { k + v.len() })
  }
  for f in fs {
    t += f()
  }
  t
}

fn walking(m: Map[Int, Str]) -> Int {
  var t = 0
  for (k, v) in m {
    t += k + v.len()
  }
  t
}

fn main(os: Os) {
  let (m, l) = log_map(20)
  os.stdio.println("${copying(m)} ${walking(m)} ${l}")
}
`

// TestVetCostMapCopy: `xo vet --cost` notes the loop that copies a
// log-mode map's entries (XO0907) and not the one that walks it in
// place; `xo vet` alone reports no cost notes (decision 0327).
func TestVetCostMapCopy(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostProg})
	lines := func(args ...string) []int {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := run(t, append([]string{"vet", "--json"}, append(args, dir)...), &out, &errOut); code != 0 {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		var res struct {
			Diagnostics []struct {
				Code string `json:"code"`
				Span struct {
					Line int `json:"line"`
				} `json:"span"`
			} `json:"diagnostics"`
		}
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("bad JSON: %v\n%s", err, out.String())
		}
		var ls []int
		for _, d := range res.Diagnostics {
			if d.Code == "XO0907" {
				ls = append(ls, d.Span.Line)
			}
		}
		return ls
	}
	if got := lines("--cost"); len(got) != 1 || got[0] != 23 {
		t.Errorf("vet --cost: XO0907 at lines %v, want [23]", got)
	}
	if got := lines(); len(got) != 0 {
		t.Errorf("vet without --cost: XO0907 at lines %v, want none", got)
	}
}

// vetCostElemProg: an element passed to a function value whose result
// has its type is handed over, so counted up and down each iteration
// (XO0908, line 3); passed to a function value that borrows it (decision
// 0337, line 11), to a named function that only reads it, or pushed into
// a list kept after, it is not.
const vetCostElemProg = `fn via_value(xs: List[Str], f: fn(Str) -> Str) -> Int {
  var t = 0
  for x in xs {
    t += f(x).len()
  }
  t
}

fn via_borrow(xs: List[Str], f: fn(Str) -> Int) -> Int {
  var t = 0
  for x in xs {
    t += f(x)
  }
  t
}

fn width(s: Str) -> Int {
  s.len()
}

fn direct(xs: List[Str]) -> Int {
  var t = 0
  for x in xs {
    t += width(x)
  }
  t
}

fn kept(xs: List[Str]) -> List[Str] {
  var out: List[Str] = []
  for x in xs {
    out.push(x)
  }
  out
}

fn main(os: Os) {
  let xs = ["a", "bb", "ccc"]
  os.stdio.println("${via_value(xs, fn(s) { "${s.len()}" })} ${via_borrow(xs, fn(s) { s.len() })} ${direct(xs)} ${kept(xs).len()}")
}
`

// TestVetCostElemCount: `xo vet --cost` notes the loop whose element goes
// to a function value (XO0908) and not the borrowed or stored ones.
func TestVetCostElemCount(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostElemProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0908" {
			lines = append(lines, d.Span.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 3 {
		t.Errorf("XO0908 at lines %v, want [3]\n%s", lines, out.String())
	}
}

// vetCostCallbackProg: a loop handing struct elements to a function value
// whose result is their type (line 9: their fields are retained, the
// call takes the struct) and a prelude map with such a function (line
// 52) are noted; since decision 0337 not the prelude's callback walks
// whose function value borrows the heap elements (lines 15, 19, 23), the
// same walks over Int elements or with a heap accumulator only (lines
// 27, 31), a for loop (line 35), or a loop handing the elements to a
// function value that borrows them (line 46).
const vetCostCallbackProg = `struct User {
  id: Int
  name: Str
}

fn via_value(xs: List[User], f: fn(User) -> User) -> Int {
  var t = 0
  for u in xs {
    t += f(u).id
  }
  t
}

fn named(xs: List[User], want: Str) -> Bool {
  xs.any(fn(u) { u.name == want })
}

fn evens(xs: List[User]) -> Int {
  xs.filter(fn(u) { u.id % 2 == 0 }).len()
}

fn total(xs: List[User]) -> Int {
  xs.fold(0, fn(t, u) { t + u.name.len() })
}

fn ints(xs: List[Int]) -> Bool {
  xs.any(fn(x) { x == 3 })
}

fn collect(xs: List[Int]) -> List[Int] {
  xs.fold([], fn(acc: List[Int], x: Int) -> List[Int] { acc + [x] })
}

fn by_loop(xs: List[User], want: Str) -> Bool {
  for u in xs {
    if u.name == want {
      return true
    }
  }
  false
}

fn via_borrow(xs: List[User], f: fn(User) -> Int) -> Int {
  var t = 0
  for u in xs {
    t += f(u)
  }
  t
}

fn renamed(xs: List[User]) -> Int {
  xs.map(fn(u: User) -> User { User{id: u.id, name: "x"} }).len()
}

fn main(os: Os) {
  let xs = [User{id: 1, name: "a"}, User{id: 2, name: "bb"}]
  os.stdio.println("${via_value(xs, fn(u) { User{id: u.id, name: "y"} })} ${named(xs, "a")} ${evens(xs)} ${total(xs)} ${ints([1, 3])} ${collect([1]).len()} ${by_loop(xs, "a")} ${via_borrow(xs, fn(u) { u.id })} ${renamed(xs)}")
}
`

// TestVetCostCallbacks: XO0908 notes a loop handing struct elements to a
// function value that takes them and a prelude walk whose function value
// takes them (gotchas/vet-cost-misses-callbacks.md), and not the walks
// whose function value borrows its element (decision 0337), Int elements,
// a heap accumulator alone, or a for loop.
func TestVetCostCallbacks(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostCallbackProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0908" {
			lines = append(lines, d.Span.Line)
		}
	}
	sort.Ints(lines)
	if fmt.Sprint(lines) != "[9 52]" {
		t.Errorf("XO0908 at lines %v, want [9 52]\n%s", lines, out.String())
	}
}

// vetCostLargeProg: a closure capturing a 9-word struct (XO0910, line 14)
// and an Option holding one (line 25); a closure capturing one field of
// it and an Option of its index are not noted.
const vetCostLargeProg = `struct Big {
  a: Int
  b: Int
  c: Int
  d: Int
  e: Int
  f: Int
  g: Int
  h: Int
  i: Int
}

fn adder(big: Big) -> fn(Int) -> Int {
  fn(k: Int) -> Int { big.a + k }
}

fn adder_field(big: Big) -> fn(Int) -> Int {
  let a = big.a
  fn(k: Int) -> Int { a + k }
}

fn find(xs: List[Big], n: Int) -> Big? {
  for x in xs {
    if x.a == n {
      return Some(x)
    }
  }
  None
}

fn find_index(xs: List[Big], n: Int) -> Int? {
  for (i, x) in xs.indexed() {
    if x.a == n {
      return Some(i)
    }
  }
  None
}

fn main(os: Os) {
  let big = Big{a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7, h: 8, i: 9}
  let found = find([big], 1)
  let at = find_index([big], 1)
  os.stdio.println("${adder(big)(1)} ${adder_field(big)(2)} ${found.is_some()} ${at.is_some()}")
}
`

// TestVetCostLarge: `xo vet --cost` notes a closure capturing a value
// over 8 words and an Option holding one inline (XO0910), and not their
// fixed forms.
func TestVetCostLarge(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostLargeProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0910" {
			lines = append(lines, d.Span.Line)
		}
	}
	if len(lines) != 2 || lines[0] != 14 || lines[1] != 25 {
		t.Errorf("XO0910 at lines %v, want [14 25]\n%s", lines, out.String())
	}
}

// vetCostReuseProg: a list updated while a snapshot of it is kept in the
// same iteration (XO0909, line 6); a snapshot made before the loop, a
// conditional push to a unique list, a list made in each iteration, a
// var parameter, and a copy updated under another name are not noted.
const vetCostReuseProg = `fn snap(n: Int) -> Int {
  var xs: List[Int] = []
  var snaps: List[List[Int]] = []
  for i in 0..n {
    snaps.push(xs)
    xs.push(i)
  }
  snaps.len() + xs.len()
}

fn before_loop(n: Int) -> Int {
  var ys: List[Int] = []
  let before = ys
  for i in 0..n {
    ys.push(i)
  }
  ys.len() + before.len()
}

fn conditional(n: Int) -> Int {
  var ks: List[Int] = []
  for i in 0..n {
    if i % 2 == 0 {
      ks.push(i)
    }
  }
  ks.len()
}

fn fresh(n: Int) -> Int {
  var total = 0
  for i in 0..n {
    var t: List[Int] = []
    t.push(i)
    total += t.len()
  }
  total
}

fn param(var xs: List[Int], n: Int) -> Int {
  for i in 0..n {
    xs.push(i)
  }
  xs.len()
}

fn copy_each(best: List[Int], n: Int) -> Int {
  var total = 0
  for i in 0..n {
    var cand = best
    cand.push(i)
    total += cand.len()
  }
  total
}

fn main(os: Os) {
  var e: List[Int] = []
  os.stdio.println("${snap(3)} ${before_loop(3)} ${conditional(3)} ${fresh(3)} ${param(e, 3)} ${copy_each([1], 3)}")
}
`

// TestVetCostReuse: `xo vet --cost` notes a list updated while the loop
// keeps a holder of it (XO0909), and not the shapes that copy once or
// not at all.
func TestVetCostReuse(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostReuseProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0909" {
			lines = append(lines, d.Span.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 6 {
		t.Errorf("XO0909 at lines %v, want [6]\n%s", lines, out.String())
	}
}

// vetCostViewProg: slices of a parameter's text made in a loop and stored
// in a returned list (XO0911, line 11); not the full text stored, a slice
// used and dropped, a list of slices that stays local, nor a slice cloned
// where it is stored.
const vetCostViewProg = `
struct Tok {
  name: Str
  val: Str
}

fn parse(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    let i = line.find("=") ?? 0
    out.push(Tok{name: line[0..i], val: line[i + 1..].trim()})
  }
  out
}

fn keep_full(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for _i in 0..2 {
    out.push(Tok{name: text, val: text})
  }
  out
}

fn local_only(text: Str) -> Int {
  var n = 0
  for line in text.lines() {
    let t = line[0..1]
    n += t.len()
  }
  n
}

fn local_list(text: Str) -> Int {
  var parts: List[Str] = []
  for line in text.lines() {
    parts.push(line[0..1])
  }
  parts.join(",").len()
}

fn cloned(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    let i = line.find("=") ?? 0
    out.push(Tok{name: line[0..i].clone(), val: line[i + 1..].trim().clone()})
  }
  out
}

fn main(os: Os) {
  let ts = parse("a=b\nc=d")
  os.stdio.println("${ts.len()} ${keep_full("x").len()} ${local_only("ab")} ${local_list("ab")} ${cloned("a=b").len()}")
}
`

// TestVetCostViewStore: `xo vet --cost` notes a view of a parameter's
// text stored in a returned collection (XO0911, decision 0330), and not
// the full text, a slice used and dropped, a list that stays local, or a
// slice cloned where it is stored.
func TestVetCostViewStore(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostViewProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0911" {
			lines = append(lines, d.Span.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 11 {
		t.Errorf("XO0911 at lines %v, want [11]\n%s", lines, out.String())
	}
}

// vetCostViewOptionProg: a slice of a parameter's text wrapped in an
// Option and unwrapped with ?? before it is stored in a returned list
// (XO0911, line 9: the analysis follows the Option's payload, the shape of
// `line.cut_prefix("x") ?? ""`); the same unwrapped view returned alone is
// not noted.
const vetCostViewOptionProg = `
struct Tok {
  name: Str
}

fn parse(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    let o: Str? = if line.len() > 2 { Some(line[1..]) } else { None }
    out.push(Tok{name: o ?? ""})
  }
  out
}

fn first(text: Str) -> Str {
  for line in text.lines() {
    let o: Str? = if line.len() > 2 { Some(line[1..]) } else { None }
    return o ?? ""
  }
  ""
}

fn main(os: Os) {
  os.stdio.println("${parse("ab\ncd").len()} ${first("abc")}")
}
`

// TestVetCostViewStoreOption: XO0911 follows a view through an Option
// unwrapped with ?? into a returned collection (lead request 2026-10-23,
// found by xo-deepseek-1 with cut_prefix).
func TestVetCostViewStoreOption(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostViewOptionProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0911" {
			lines = append(lines, d.Span.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 9 {
		t.Errorf("XO0911 at lines %v, want [9]\n%s", lines, out.String())
	}
}

// vetCostSplitNProg: a piece of a split_n of a line (itself a piece of the
// parameter's text) stored in a struct inside a loop (XO0911, line 9); the
// same piece returned alone is not noted.
const vetCostSplitNProg = `
struct Tok {
  name: Str
}

fn parse(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    out.push(Tok{name: line.split_n(",", 2)[0]})
  }
  out
}

fn first(text: Str) -> Str {
  for line in text.lines() {
    return line.split_n(",", 2)[0]
  }
  ""
}

fn main(os: Os) {
  os.stdio.println("${parse("ab,cd\nef").len()} ${first("ab,cd")}")
}
`

// TestVetCostViewStoreSplitN: XO0911 names split_n, split_after and
// split_after_n beside split in the parameter-view list
// (compiler/ssa/cost.xo param_view_at), on the shape e6a66135b covers for
// Options: a piece of a split stored in a struct inside a loop.
func TestVetCostViewStoreSplitN(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostSplitNProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0911" {
			lines = append(lines, d.Span.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 9 {
		t.Errorf("XO0911 at lines %v, want [9]\n%s", lines, out.String())
	}
}

// vetCostViewCutProg: the same shape with Str.cut_prefix, so the three cut
// functions' place in view_call (compiler/ssa/cost.xo) has a test: without
// it the note below disappears (checked by removing them).
const vetCostViewCutProg = `
struct Tok {
  name: Str
}

fn parse(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    out.push(Tok{name: line.cut_prefix("x") ?? ""})
  }
  out
}

fn main(os: Os) {
  os.stdio.println("${parse("x1\nx2").len()}")
}
`

// TestVetCostViewStoreCutOption: XO0911 follows line.cut_prefix(x) ?? ""
// into a field, which needs str_cut_prefix in view_call.
func TestVetCostViewStoreCutOption(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostViewCutProg})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0911" {
			lines = append(lines, d.Span.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 9 {
		t.Errorf("XO0911 at lines %v, want [9]\n%s", lines, out.String())
	}
}

// vetCostNotes runs xo vet --cost on prog and returns the lines of its
// XO0911 notes, in order.
func vetCostNotes(t *testing.T, prog string) []int {
	t.Helper()
	dir := writeFiles(t, map[string]string{"main.xo": prog})
	var out, errOut bytes.Buffer
	if status := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); status != 0 {
		t.Fatalf("exit %d: %s", status, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0911" {
			lines = append(lines, d.Span.Line)
		}
	}
	return lines
}

// vetCostViewTwoPushesProg: two split_n pieces of a parameter's text, each
// stored in a struct and both pushed into a returned list. Each push makes a
// new list, so the earlier piece is only reachable through the next push's
// collection argument (XO0911 at lines 8 and 9).
const vetCostViewTwoPushesProg = `
struct Tok {
  name: Str
}

fn two(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  out.push(Tok{name: text.split_n(",", 2)[0]})
  out.push(Tok{name: text.split_n(",", 3)[0]})
  out
}

fn main(os: Os) {
  os.stdio.println("${two("a,b").len()}")
}
`

// TestVetCostViewStoreTwoPushes: XO0911 follows a stored view through the
// collection argument of a later push (Xo Lang 1's approval of the fix,
// 2026-10-23): both pushes are noted, not only the last.
func TestVetCostViewStoreTwoPushes(t *testing.T) {
	t.Parallel()
	lines := vetCostNotes(t, vetCostViewTwoPushesProg)
	if len(lines) != 2 || lines[0] != 8 || lines[1] != 9 {
		t.Errorf("XO0911 at lines %v, want [8 9]", lines)
	}
}

// vetCostViewMapPutProg: two split_n pieces of a parameter's text put into a
// returned map. As with push, the earlier put's map is the next put's
// collection argument (XO0911 at lines 4 and 5).
const vetCostViewMapPutProg = `
fn picks(text: Str) -> Map[Str, Str] {
  var out: Map[Str, Str] = {}
  out.put("a", text.split_n(",", 2)[0])
  out.put("b", text.split_n(",", 3)[0])
  out
}

fn main(os: Os) {
  os.stdio.println("${picks("a,b").len()}")
}
`

// TestVetCostViewStoreMapPut: the fix covers map.put's collection argument
// as well as list.push's.
func TestVetCostViewStoreMapPut(t *testing.T) {
	t.Parallel()
	lines := vetCostNotes(t, vetCostViewMapPutProg)
	if len(lines) != 2 || lines[0] != 4 || lines[1] != 5 {
		t.Errorf("XO0911 at lines %v, want [4 5]", lines)
	}
}

// TestLookupsCompactText covers decision 0386: with standard output not a
// terminal the lookups print text a record a line (they printed one line
// of JSON), symbols takes a NAME filter and --limit, refs --limit and a
// context per reference, and --json is the same JSON broken over lines.
func TestLookupsCompactText(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module ex\n",
		"main.xo": "/// Adds one.\n/// The second doc line.\nfn add_one(n: Int) -> Int { n + 1 }\n\nfn add_two(n: Int) -> Int { add_one(add_one(n)) }\n\nfn other() -> Int { 3 }\n\nfn main(os: Os) {\n  os.stdio.println(\"${add_two(1)} ${other()}\")\n}\n",
	})
	lookup := func(args ...string) string {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := run(t, append(args, "--dir="+dir), &out, &errOut); code != 0 {
			t.Fatalf("xo %v: exit %d\n%s%s", args, code, out.String(), errOut.String())
		}
		return out.String()
	}
	syms := lookup("symbols")
	if strings.HasPrefix(syms, "{") || !strings.Contains(syms, "func add_one  "+filepath.Join(dir, "main.xo")+":3  fn add_one(n: Int) -> Int\n") || strings.Contains(syms, "Adds one") {
		t.Errorf("symbols text:\n%s", syms)
	}
	if got := lookup("symbols", "add_*"); strings.Count(got, "\n") != 2 || strings.Contains(got, "other") {
		t.Errorf("symbols add_*:\n%s", got)
	}
	if got := lookup("symbols", "--limit=1"); !strings.Contains(got, "note: showing 1 of ") {
		t.Errorf("symbols --limit=1:\n%s", got)
	}
	if got := lookup("symbols", "--doc", "add_one"); !strings.Contains(got, "  /// The second doc line.") {
		t.Errorf("symbols --doc:\n%s", got)
	}
	refs := lookup("refs", "add_one")
	if !strings.Contains(refs, "main.xo:5:29  fn add_two(n: Int) -> Int { add_one(add_one(n)) }") || !strings.Contains(refs, "3 reference(s)") {
		t.Errorf("refs text:\n%s", refs)
	}
	if got := lookup("refs", "add_one", "--limit=1"); !strings.Contains(got, "note: showing 1 of 3 references") || !strings.Contains(got, "3 reference(s)") {
		t.Errorf("refs --limit=1:\n%s", got)
	}
	def := lookup("def", "add_one")
	if !strings.Contains(def, "/// Adds one.") || strings.Contains(def, "second doc line") {
		t.Errorf("def text (the doc's first line only):\n%s", def)
	}
	js := lookup("refs", "add_one", "--json")
	var v struct {
		References []struct{ File string } `json:"references"`
	}
	if err := json.Unmarshal([]byte(js), &v); err != nil || len(v.References) != 3 || strings.Count(js, "\n") < 4 {
		t.Errorf("refs --json (%v, %d references):\n%s", err, len(v.References), js)
	}
}

// TestQueryStdModuleOutsideModule: `xo query api std/json` in a directory
// with no module loads the std module, as def and callers do, and lists
// its surface (std's declarations carry no pub); `deps std/json` says
// std packages are not followed (they printed an empty list and "no
// package ... among the checked modules", 2026-10-26).
func TestQueryStdModuleOutsideModule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	if code := run(t, []string{"query", "api", "--dir=" + dir, "--text", "std/json"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "fn decode") {
		t.Errorf("query api std/json outside a module: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"query", "deps", "--dir=" + dir, "--text", "std/json"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "is a std module") {
		t.Errorf("query deps std/json: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}

// vetCostLoopProg: a Str and a Bytes slice in a loop read only by index,
// length or equality (XO0913, lines 6 and 19); a four element list made
// and only read each iteration (XO0914, line 29); a list kept past its
// iteration is not noted (line 38).
const vetCostLoopProg = `
fn count_ab(s: Str, p: Str) -> Int {
  var n = 0
  var i = 0
  while i + p.len() <= s.len() {
    let w = s[i..i + p.len()]
    if w == p {
      n += 1
    }
    i += 1
  }
  n
}

fn first_bytes(b: Bytes) -> Int {
  var t = 0
  var i = 0
  while i + 4 <= b.len() {
    let q = b[i..i + 4]
    t += Int(q[0]) + Int(q[3]) + q.len()
    i += 4
  }
  t
}

fn quads(xs: List[Int]) -> Int {
  var t = 0
  for x in xs {
    let q = [x, x + 1, x + 2, x + 3]
    t += q[0] + q[3] + q.len()
  }
  t
}

fn kept(xs: List[Int]) -> List[List[Int]] {
  var out: List[List[Int]] = []
  for x in xs {
    let q = [x, x + 1]
    out.push(q)
  }
  out
}

fn main(os: Os) {
  os.stdio.println("${count_ab("abab", "ab")} ${first_bytes(b"abcdefgh")} ${quads([1, 2])} ${kept([1]).len()}")
}
`

// vetCostCodes runs xo vet --cost --json over src and returns the lines
// of each code's notes.
func vetCostCodes(t *testing.T, src string) map[string][]int {
	t.Helper()
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	got := map[string][]int{}
	for _, d := range res.Diagnostics {
		got[d.Code] = append(got[d.Code], d.Span.Line)
	}
	return got
}

// TestVetCostLoopSlicesAndLists: XO0913 and XO0914 (the std timing audit's
// lessons, knowledgebase/std-performance-guide.md 3 and 4).
func TestVetCostLoopSlicesAndLists(t *testing.T) {
	t.Parallel()
	got := vetCostCodes(t, vetCostLoopProg)
	if fmt.Sprint(got["XO0913"]) != "[6 19]" {
		t.Errorf("XO0913 at lines %v, want [6 19]", got["XO0913"])
	}
	if fmt.Sprint(got["XO0914"]) != "[29]" {
		t.Errorf("XO0914 at lines %v, want [29]", got["XO0914"])
	}
}
