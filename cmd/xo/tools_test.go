package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	for _, c := range []string{"XO0703", "XO0900", "XO0901"} {
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
