package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestLSPStdDeclarations: a std source opened in the editor is never
// checked as user code. Opening a release's lib/xo/std/charset.xo (a
// declaration file, its fns bodyless) checked its directory as a package:
// every declaration was "expected `{` to start the body", and the walk
// put problems on most of std's files. The file of another release's std
// tree, a file of the checker's own std, and a native implementation file
// all get no diagnostics, while a user module beside them still does.
func TestLSPStdDeclarations(t *testing.T) {
	t.Parallel()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	// another release's std tree: prelude.xo and VERSION mark its root
	other := filepath.Join(t.TempDir(), "tools", "xo-v0.44.0-darwin-arm64", "lib", "xo", "std")
	for _, f := range []string{"prelude.xo", "VERSION", "charset.xo", "json.xo", "native/charset.xo"} {
		b, err := os.ReadFile(filepath.Join(root, "std", filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(other, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// a user module whose files sit beside a prelude.xo and VERSION of its
	// own: its xo.mod makes it user code
	bad := "fn main(os: Os) {\n  let x: Int = \"no\"\n  os.stdio.println(\"${x}\")\n}\n"
	dir := writeFiles(t, map[string]string{
		"xo.mod":     "module example.com/lspstd\nedition preview\n",
		"prelude.xo": "fn helper() -> Int {\n  1\n}\n",
		"VERSION":    "0.0.0\n",
		"main.xo":    bad,
	})
	c := startLSP(t, dir)
	c.request("initialize", map[string]any{"processId": nil, "rootUri": fileURL(dir), "capabilities": map[string]any{}})
	c.notify("initialized", map[string]any{})
	open := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		uri := fileURL(path)
		c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "xo", "version": 1, "text": string(b)}})
		return uri
	}
	var std []string
	for _, f := range []string{
		filepath.Join(other, "charset.xo"),
		filepath.Join(other, "json.xo"),
		filepath.Join(other, "native", "charset.xo"),
		filepath.Join(root, "std", "charset.xo"),
	} {
		std = append(std, open(f))
	}
	mainURI := open(filepath.Join(dir, "main.xo"))
	// every publish covers every open document: read them until main.xo's
	// type error, keeping each uri's last
	last := map[string][]any{}
	timeout := time.After(3 * time.Minute)
	for len(last[mainURI]) == 0 {
		select {
		case msg, ok := <-c.msgs:
			if !ok {
				t.Fatal("server closed its output")
			}
			if msg["method"] != "textDocument/publishDiagnostics" {
				continue
			}
			p, _ := msg["params"].(map[string]any)
			ds, _ := p["diagnostics"].([]any)
			last[fmt.Sprint(p["uri"])] = ds
		case <-timeout:
			t.Fatalf("no diagnostics of main.xo, a user module beside a prelude.xo (want its type error); got %v", last)
		}
	}
	for _, uri := range std {
		if d, ok := last[uri]; ok && len(d) != 0 {
			t.Errorf("%s: %d diagnostics, want none (a std source): %v", uri, len(d), d[0])
		}
	}
	for uri := range last {
		if uri != mainURI && !slices.Contains(std, uri) {
			t.Errorf("diagnostics published for %s, a file no one opened", uri)
		}
	}
}

// TestLSPStdImplementation: go to definition (and peek, and type
// definition) on a std name answers its implementation in std's native
// sources where it has one, not the declaration file's bodyless
// signature (owner, 2026-11-10: Str.split_where opened
// lsp-std/std/prelude.xo). A method found as a flat fn (Str.split_where is
// str_split_where in std/native/prelude.xo), a std type (http.Request, a
// struct in std/native/http.xo, by definition and by type definition of
// a local), and a name with no implementation (Str.len, an intrinsic)
// that keeps its declaration.
func TestLSPStdImplementation(t *testing.T) {
	t.Parallel()
	src := "use std/http\n\nfn main(os: Os) {\n  let parts = \"a b\".split_where(fn(r) { r == ' ' })\n  let req: http.Request = http.new_request(\"GET\", \"/\")\n  os.stdio.println(\"${parts.len()} ${\"ab\".len()} ${req.path}\")\n}\n"
	dir := writeFiles(t, map[string]string{"xo.mod": "module example.com/lspimpl\nedition preview\n", "main.xo": src})
	uri := fileURL(filepath.Join(dir, "main.xo"))
	c := startLSP(t, dir)
	initRes, _ := c.request("initialize", map[string]any{"processId": nil, "rootUri": fileURL(dir), "capabilities": map[string]any{}}).(map[string]any)
	caps, _ := initRes["capabilities"].(map[string]any)
	if caps["typeDefinitionProvider"] != true {
		t.Errorf("typeDefinitionProvider = %v, want true", caps["typeDefinitionProvider"])
	}
	c.notify("initialized", map[string]any{})
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "xo", "version": 1, "text": src}})
	// the location's file and the text of its line
	at := func(method, after string) (string, string) {
		t.Helper()
		l, ch := lspAt(t, src, after)
		loc, _ := c.request(method, lspPos(uri, l, ch)).(map[string]any)
		u, _ := loc["uri"].(string)
		if u == "" {
			t.Errorf("%s after %q = %v, want a location", method, after, loc)
			return "", ""
		}
		path := filepath.FromSlash(strings.TrimPrefix(u, "file://"))
		if len(path) > 2 && path[0] == '\\' && path[2] == ':' {
			path = path[1:] // file:///C:/x
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		r, _ := loc["range"].(map[string]any)
		start, _ := r["start"].(map[string]any)
		line, _ := start["line"].(float64)
		lines := strings.Split(string(b), "\n")
		if int(line) >= len(lines) {
			t.Fatalf("%s: line %v past the end", u, line)
		}
		return filepath.ToSlash(path), lines[int(line)]
	}
	for _, c := range []struct{ method, after, file, line string }{
		{"textDocument/definition", "\".split_wh", "/std/native/prelude.xo", "fn str_split_where["},
		{"textDocument/definition", "http.Requ", "/std/native/http.xo", "struct Request"},
		{"textDocument/typeDefinition", "let re", "/std/native/http.xo", "struct Request"},
		{"textDocument/definition", "\"ab\".le", "/std/prelude.xo", "fn (s: Str) len()"},
	} {
		file, line := at(c.method, c.after)
		if !strings.HasSuffix(file, c.file) || !strings.Contains(line, c.line) {
			t.Errorf("%s after %q = %s %q, want %s with %q", c.method, c.after, file, line, c.file, c.line)
		}
	}
}
