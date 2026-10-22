package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestParseOK(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"a.xo":       "fn main(os: Os) ! {}\n",
		"sub/b.xo":   "struct S { a: Int }\n",
		"ignored.go": "package x\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"parse", dir + "/..."}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "2 file(s), 0 error(s)") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestParseErrorsHuman(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"bad.xo": "fn f() {\n  let = 1\n}\nlet y = 2\n"})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"parse", dir}, &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	s := errOut.String()
	if !strings.Contains(s, "bad.xo:2:7: error XO0165:") || !strings.Contains(s, "bad.xo:4:1: error XO0168:") {
		t.Errorf("stderr = %s", s)
	}
}

func TestParseJSON(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"bad.xo": "fn f() {\n  g(\n    a\n  )\n}\n"})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"parse", "--json", "--ast", dir}, &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	var res struct {
		Files []struct {
			Path string         `json:"path"`
			AST  map[string]any `json:"ast"`
		} `json:"files"`
		Diagnostics []struct {
			Code  string `json:"code"`
			Fixes []struct {
				Title string `json:"title"`
			} `json:"fixes"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if len(res.Files) != 1 || res.Files[0].AST["kind"] != "File" {
		t.Errorf("files = %+v", res.Files)
	}
	if len(res.Diagnostics) != 1 || res.Diagnostics[0].Code != "XO0161" || len(res.Diagnostics[0].Fixes) != 1 {
		t.Errorf("diagnostics = %+v", res.Diagnostics)
	}
}

func TestParseASTOnly(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"a.xo": "const X = 1\n"})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"parse", "--ast", filepath.Join(dir, "a.xo")}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"kind": "ConstDecl"`) {
		t.Errorf("stdout = %s", out.String())
	}
}

func TestUsage(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run(t, nil, &out, &errOut); code != 2 {
		t.Errorf("no args: exit %d", code)
	}
	if code := run(t, []string{"parse"}, &out, &errOut); code != 2 {
		t.Errorf("parse without files: exit %d", code)
	}
	if code := run(t, []string{"frobnicate"}, &out, &errOut); code != 2 {
		t.Errorf("unknown command: exit %d", code)
	}
	if code := run(t, []string{"parse", "/does/not/exist"}, &out, &errOut); code != 2 {
		t.Errorf("missing path: exit %d", code)
	}
}

func TestCheckJSON(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/main.xo": "enum E { A }\nfn g() -> Int ! E { 1 }\nfn main(os: Os) {\n  g()\n}\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", "--json", dir + "/..."}, &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1: %s", code, errOut.String())
	}
	var res struct {
		Modules     []string `json:"modules"`
		Diagnostics []struct {
			Code  string `json:"code"`
			Notes []any  `json:"notes"`
			Fixes []struct {
				Title string `json:"title"`
			} `json:"fixes"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if len(res.Modules) != 1 || len(res.Diagnostics) != 1 || res.Diagnostics[0].Code != "XO0201" || len(res.Diagnostics[0].Fixes) == 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestCheckOK(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"app/x.go":    "package x\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", dir + "/..."}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "1 module(s), 0 error(s), 0 warning(s)") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestCheckUsage(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check"}, &out, &errOut); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if code := run(t, []string{"check", "/does/not/exist"}, &out, &errOut); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestBuildRunTest drives `xo build`, `xo run`, and `xo test` on a small
// program.
func TestBuildRunTest(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds Go")
	}
	dir := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	src := filepath.Join(dir, "app")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	prog := "fn double(n: Int) -> Int { n * 2 }\n\nfn main(os: Os) {\n  os.stdio.println(\"${double(21)} ${os.env.args().join(\",\")}\")\n}\n\ntest \"double\" {\n  expect(double(2)) == 4\n}\n\ntest \"wrong\" {\n  expect(double(2)) == 5\n}\n"
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	bin := filepath.Join(dir, "app.bin")
	if code := run(t, []string{"build", "-o", bin, src}, &out, &errOut); code != 0 {
		t.Fatalf("build: exit %d: %s", code, errOut.String())
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("no binary: %v", err)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"run", src, "--", "a", "b"}, &out, &errOut); code != 0 || out.String() != "42 a,b\n" {
		t.Fatalf("run: exit %d stdout %q stderr %s", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := run(t, []string{"test", src}, &out, &errOut); code != 1 || !strings.Contains(out.String(), "FAIL: 1 passed, 1 failed") {
		t.Fatalf("test: exit %d stdout %s", code, out.String())
	}
	out.Reset()
	if code := run(t, []string{"test", "--run", "double", src}, &out, &errOut); code != 0 {
		t.Fatalf("test --run: exit %d stdout %s", code, out.String())
	}
}
