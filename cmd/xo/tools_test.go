package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if code := run(t, []string{"fmt", "--check", dir}, &out, &errOut); code != 1 {
		t.Fatalf("--check exit %d, want 1: %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "a.xo") || !strings.Contains(got, "c.xo") || strings.Contains(got, "b.xo") || strings.Contains(got, "skip.xo") {
		t.Errorf("--check listed:\n%s", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.xo")); string(data) != "fn f(a:Int)->Int{\n    a+1\n}\n" {
		t.Errorf("--check changed a file")
	}
	out.Reset()
	if code := run(t, []string{"fmt", dir}, &out, &errOut); code != 0 {
		t.Fatalf("fmt exit %d: %s", code, errOut.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.xo")); string(data) != "fn f(a: Int) -> Int {\n  a + 1\n}\n" {
		t.Errorf("a.xo = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".xo/skip.xo")); string(data) != "fn  skipped( ) {}\n" {
		t.Errorf("dot directory was formatted")
	}
	out.Reset()
	if code := run(t, []string{"fmt", "--check", dir}, &out, &errOut); code != 0 || out.Len() != 0 {
		t.Errorf("second --check: exit %d, out %q", code, out.String())
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
func agentJSON(t *testing.T, code int, args ...string) map[string]any {
	t.Helper()
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
	for _, args := range [][]string{{"--std-module=native/llrt/pool"}, {"--std-module=json"}, {filepath.Join(repoRoot, "std", "native", "llrt")}} {
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
