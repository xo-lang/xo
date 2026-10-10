package lsp

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testProg = `use std/log

/// A point on the grid.
struct Point {
  x: Int
  y: Int
}

const ORIGIN_X = 0

/// Sum of the coordinates.
fn (p: Point) sum() -> Int {
  p.x + p.y
}

fn make(x: Int) -> Point {
  Point{x: x, y: ORIGIN_X}
}

fn main() {
  let p = make(1)
  let total = p.sum()
  log.info("total ${total}")
}
`

// client drives a Server over pipes.
type client struct {
	t     *testing.T
	in    *io.PipeWriter
	out   *bufio.Reader
	next  int
	notes chan *message
	resps chan *message
	done  chan error
}

func start(t *testing.T) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &Server{Version: "test", Delay: time.Millisecond, StdDir: t.TempDir()}
	c := &client{t: t, in: inW, out: bufio.NewReader(outR), notes: make(chan *message, 100), resps: make(chan *message, 10), done: make(chan error, 1)}
	go func() { c.done <- s.Serve(inR, outW); outW.Close() }()
	go func() {
		for {
			m, err := readMessage(c.out)
			if err != nil {
				return
			}
			if m.ID != nil {
				c.resps <- m
			} else {
				c.notes <- m
			}
		}
	}()
	t.Cleanup(func() {
		c.notify("exit", nil)
		select {
		case err := <-c.done:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not exit")
		}
	})
	c.call("initialize", map[string]any{"capabilities": map[string]any{}}, nil)
	c.notify("initialized", map[string]any{})
	return c
}

func (c *client) send(m map[string]any) {
	m["jsonrpc"] = "2.0"
	body, _ := json.Marshal(m)
	if _, err := io.WriteString(c.in, "Content-Length: "+itoa(len(body))+"\r\n\r\n"+string(body)); err != nil {
		c.t.Fatal(err)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func (c *client) notify(method string, params any) {
	m := map[string]any{"method": method}
	if params != nil {
		m["params"] = params
	}
	c.send(m)
}

// call sends a request and decodes its result into out; it fails the test
// on an error response.
func (c *client) call(method string, params, out any) {
	c.t.Helper()
	if err := c.try(method, params, out); err != nil {
		c.t.Fatalf("%s: %v", method, err)
	}
}

func (c *client) try(method string, params, out any) error {
	c.t.Helper()
	c.next++
	c.send(map[string]any{"id": c.next, "method": method, "params": params})
	select {
	case m := <-c.resps:
		if m.Error != nil {
			return m.Error
		}
		if out != nil {
			if err := json.Unmarshal(m.Result, out); err != nil {
				c.t.Fatalf("%s result %s: %v", method, m.Result, err)
			}
		}
		return nil
	case <-time.After(10 * time.Second):
		c.t.Fatalf("%s: no response", method)
	}
	return nil
}

// diagnostics waits for the next publishDiagnostics for uri.
func (c *client) diagnostics(uri string) []lspDiagnostic {
	c.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m := <-c.notes:
			if m.Method != "textDocument/publishDiagnostics" {
				continue
			}
			var p publishDiagnosticsParams
			json.Unmarshal(m.Params, &p)
			if p.URI == uri {
				return p.Diagnostics
			}
		case <-deadline:
			c.t.Fatalf("no diagnostics for %s", uri)
		}
	}
}

func open(t *testing.T, src string) (*client, string) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.xo")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c := start(t)
	uri := pathToURI(path)
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "xo", "version": 1, "text": src}})
	return c, uri
}

// pos is the position of the n-th byte of the first occurrence of needle
// (ASCII sources only).
func pos(t *testing.T, src, needle string, n int) position {
	t.Helper()
	i := strings.Index(src, needle)
	if i < 0 {
		t.Fatalf("%q not in source", needle)
	}
	off := i + n
	line := strings.Count(src[:off], "\n")
	return position{Line: line, Character: off - (strings.LastIndex(src[:off], "\n") + 1)}
}

func lineOf(src, needle string) int { return strings.Count(src[:strings.Index(src, needle)], "\n") }

func docPos(uri string, p position) map[string]any {
	return map[string]any{"textDocument": map[string]any{"uri": uri}, "position": p}
}

func TestDiagnosticsAndQuickFix(t *testing.T) {
	c, uri := open(t, testProg)
	if ds := c.diagnostics(uri); len(ds) != 0 {
		t.Fatalf("clean program has diagnostics: %+v", ds)
	}
	bad := strings.Replace(testProg, "  let total = p.sum()\n", "  let total = p.sum()\n  let unused: Str = 1\n", 1)
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{{"text": bad}},
	})
	ds := c.diagnostics(uri)
	codes := map[string]lspDiagnostic{}
	for _, d := range ds {
		codes[d.Code] = d
	}
	d, ok := codes["XO0401"]
	if !ok {
		t.Fatalf("want XO0401 (type mismatch), got %+v", ds)
	}
	if want := pos(t, bad, "= 1\n", 2); d.Range.Start != want || d.Severity != sevError {
		t.Errorf("XO0401 at %+v severity %d, want %+v error", d.Range.Start, d.Severity, want)
	}
	u, ok := codes["XO0110"]
	if !ok {
		t.Fatalf("want XO0110 (unused), got %+v", ds)
	}
	var actions []codeAction
	c.call("textDocument/codeAction", map[string]any{"textDocument": map[string]any{"uri": uri}, "range": u.Range, "context": map[string]any{"diagnostics": []any{}}}, &actions)
	var fix *codeAction
	for i := range actions {
		if strings.Contains(actions[i].Title, "_unused") {
			fix = &actions[i]
		}
	}
	if fix == nil || fix.Edit == nil || len(fix.Edit.Changes[uri]) != 1 || fix.Edit.Changes[uri][0].NewText != "_unused" {
		t.Fatalf("want a quick fix renaming to _unused, got %+v", actions)
	}

	// an incremental change that deletes the bad line clears the diagnostics
	start := pos(t, bad, "  let unused", 0)
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 3},
		"contentChanges": []map[string]any{{"range": lspRange{Start: start, End: position{Line: start.Line + 1}}, "text": ""}},
	})
	if ds := c.diagnostics(uri); len(ds) != 0 {
		t.Fatalf("after the incremental fix: %+v", ds)
	}
}

func TestHover(t *testing.T) {
	c, uri := open(t, testProg)
	tests := []struct {
		needle string
		n      int
		want   []string
	}{
		{"make(1)", 0, []string{"fn make(x: Int) -> Point"}},
		{"total ${", 8, []string{"let total: Int"}},
		{"Point {", 0, []string{"struct Point", "A point on the grid."}},
		{"sum()\n  log", 0, []string{"fn (p: Point) sum() -> Int", "Sum of the coordinates."}},
		{"ORIGIN_X}", 0, []string{"const ORIGIN_X"}},
		{"x + p.y", 0, []string{"x: Int"}},
		{"log.info", 0, []string{"use std/log"}},
	}
	for _, tt := range tests {
		var h hover
		c.call("textDocument/hover", docPos(uri, pos(t, testProg, tt.needle, tt.n)), &h)
		for _, w := range tt.want {
			if !strings.Contains(h.Contents.Value, w) {
				t.Errorf("hover %q: want %q in %q", tt.needle, w, h.Contents.Value)
			}
		}
	}
}

func TestDefinition(t *testing.T) {
	c, uri := open(t, testProg)
	tests := []struct {
		needle string
		n      int
		line   int // -1: in std
	}{
		{"sum()\n  log", 0, lineOf(testProg, "fn (p: Point) sum")},
		{"ORIGIN_X}", 0, lineOf(testProg, "const ORIGIN_X")},
		{"x + p.y", 0, lineOf(testProg, "  x: Int")},
		{"make(1)", 0, lineOf(testProg, "fn make")},
		{"Point{x", 0, lineOf(testProg, "struct Point")},
		{"p.sum", 0, lineOf(testProg, "let p =")},
		{"log.info", 0, 0},
		{"info(", 0, -1},
	}
	for _, tt := range tests {
		var loc *location
		c.call("textDocument/definition", docPos(uri, pos(t, testProg, tt.needle, tt.n)), &loc)
		if loc == nil {
			t.Errorf("definition %q: none", tt.needle)
			continue
		}
		if tt.line < 0 {
			p := uriToPath(loc.URI)
			src, err := os.ReadFile(p)
			if !strings.HasSuffix(p, filepath.FromSlash("std/log.xo")) || err != nil {
				t.Errorf("definition %q: want std/log.xo written out, got %s (%v)", tt.needle, loc.URI, err)
				continue
			}
			if l := strings.Split(string(src), "\n")[loc.Range.Start.Line]; !strings.Contains(l, "info") {
				t.Errorf("definition %q: line %q", tt.needle, l)
			}
			continue
		}
		if loc.URI != uri || loc.Range.Start.Line != tt.line {
			t.Errorf("definition %q: %s line %d, want line %d", tt.needle, loc.URI, loc.Range.Start.Line, tt.line)
		}
	}
}

func TestReferencesAndRename(t *testing.T) {
	c, uri := open(t, testProg)
	var refs []location
	c.call("textDocument/references", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos(t, testProg, "make(1)", 0), "context": map[string]any{"includeDeclaration": true}}, &refs)
	if len(refs) != 2 {
		t.Errorf("references of make: %+v", refs)
	}
	c.call("textDocument/references", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos(t, testProg, "p = make", 0), "context": map[string]any{"includeDeclaration": false}}, &refs)
	if len(refs) != 1 || refs[0].Range.Start != pos(t, testProg, "p.sum", 0) {
		t.Errorf("references of local p: %+v", refs)
	}
	var we workspaceEdit
	c.call("textDocument/rename", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos(t, testProg, "x: Int\n  y", 0), "newName": "col"}, &we)
	if es := we.Changes[uri]; len(es) != 3 {
		// the field, `p.x`, and `Point{x: ...}`
		t.Errorf("rename field x: %+v", es)
	}
	if err := c.try("textDocument/prepareRename", docPos(uri, pos(t, testProg, "info(", 0)), nil); err == nil {
		t.Error("prepareRename on a std function should fail")
	}
}

func TestFormatAndSymbols(t *testing.T) {
	messy := strings.Replace(testProg, "  p.x + p.y", "      p.x +   p.y", 1)
	c, uri := open(t, messy)
	var edits []textEdit
	c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": uri}, "options": map[string]any{"tabSize": 2, "insertSpaces": true}}, &edits)
	if len(edits) != 1 || edits[0].NewText != testProg {
		t.Errorf("formatting: %+v", edits)
	}
	var syms []documentSymbol
	c.call("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}}, &syms)
	var names []string
	for _, s := range syms {
		names = append(names, s.Name)
	}
	if got, want := strings.Join(names, " "), "Point ORIGIN_X Point.sum make main"; got != want {
		t.Errorf("symbols: %q, want %q", got, want)
	}
	if len(syms) > 0 && len(syms[0].Children) != 2 {
		t.Errorf("struct Point children: %+v", syms[0].Children)
	}
}

func TestSemanticTokens(t *testing.T) {
	c, uri := open(t, testProg)
	var res struct{ Data []int }
	c.call("textDocument/semanticTokens/full", map[string]any{"textDocument": map[string]any{"uri": uri}}, &res)
	if len(res.Data)%5 != 0 || len(res.Data) == 0 {
		t.Fatalf("data: %v", res.Data)
	}
	got := map[position]string{}
	line, char := 0, 0
	for i := 0; i < len(res.Data); i += 5 {
		if res.Data[i] > 0 {
			line += res.Data[i]
			char = res.Data[i+1]
		} else {
			char += res.Data[i+1]
		}
		s := tokenTypes[res.Data[i+3]]
		for b, m := range tokenModifiers {
			if res.Data[i+4]&(1<<b) != 0 {
				s += "." + m
			}
		}
		got[position{line, char}] = s
	}
	tests := []struct {
		needle string
		n      int
		want   string
	}{
		{"Point {", 0, "struct.declaration"},
		{"x: Int\n  y", 0, "property.declaration"},
		{"ORIGIN_X = 0", 0, "variable.declaration.readonly"},
		{"p: Point) sum", 0, "parameter.declaration"},
		{"sum()", 0, "method.declaration"},
		{"make(x", 0, "function.declaration"},
		{"Point{x", 0, "struct"},
		{"ORIGIN_X}", 0, "variable.readonly"},
		{"total = ", 0, "variable.declaration"},
		{"p.sum", 0, "variable"},
		{"sum()\n  log", 0, "method"},
		{"log.info", 0, "namespace.defaultLibrary"},
		{"info(", 0, "function.defaultLibrary"},
		{"Int\n  y", 0, "struct.defaultLibrary"},
	}
	for _, tt := range tests {
		p := pos(t, testProg, tt.needle, tt.n)
		if got[p] != tt.want {
			t.Errorf("token at %q: %q, want %q", tt.needle, got[p], tt.want)
		}
	}
}

func TestText(t *testing.T) {
	// é is 2 bytes and 1 UTF-16 unit; 😀 is 4 bytes and 2 units
	tx := newText([]byte("let é = \"😀\" + x\nnext"))
	if p := tx.position(1, 16); p != (position{0, 12}) {
		t.Errorf("position of x: %+v", p)
	}
	if off := tx.offset(position{0, 12}); off != 15 {
		t.Errorf("offset of x: %d", off)
	}
	if l, c := tx.col(position{1, 2}); l != 2 || c != 3 {
		t.Errorf("col: %d:%d", l, c)
	}
	if p := tx.end(); p != (position{1, 4}) {
		t.Errorf("end: %+v", p)
	}
	if got := uriToPath(pathToURI("/tmp/a b/c.xo")); got != filepath.FromSlash("/tmp/a b/c.xo") {
		t.Errorf("uri round trip: %q", got)
	}
}

// TestCorpus runs every per-identifier feature over each module under the
// corpus roots and fails on a panic. XO_LSP_CORPUS adds roots (separated
// by the list separator), such as the whole repository.
func TestCorpus(t *testing.T) {
	roots := []string{"../../examples", "../../selfhost"}
	if extra := os.Getenv("XO_LSP_CORPUS"); extra != "" {
		roots = append(roots, filepath.SplitList(extra)...)
	}
	dirs := map[string][]string{}
	for _, root := range roots {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "runs" || strings.HasPrefix(d.Name(), ".") && p != root) {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(p, ".xo") {
				dirs[filepath.Dir(p)] = append(dirs[filepath.Dir(p)], p)
			}
			return nil
		})
	}
	if len(dirs) == 0 {
		t.Fatal("empty corpus")
	}
	for dir, files := range dirs {
		s := &Server{StdDir: t.TempDir(), w: &writer{w: io.Discard}, docs: map[string][]byte{}, published: map[string]bool{}}
		for _, p := range files {
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			s.docs[absPath(p)] = src
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic: %v", dir, r)
				}
			}()
			snap := s.fresh()
			for p := range s.docs {
				uri := pathToURI(p)
				s.semanticTokens(documentParams{TextDocument: textDocumentIdentifier{URI: uri}})
				s.symbols(documentParams{TextDocument: textDocumentIdentifier{URI: uri}})
				f, tx := snap.file(p)
				if f == nil {
					continue
				}
				for _, site := range snap.identsOf(f) {
					if site.id.Name == "" {
						continue
					}
					at := textDocumentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: tx.offsetPosition(site.id.Sp.Start.Offset)}
					s.hover(at)
					s.definition(at)
				}
			}
		}()
	}
	t.Logf("%d modules", len(dirs))
}

// TestStdCopy opens the std source a definition led to: it must not be
// checked as user code (std files have no bodies), and requests in it are
// answered from the std module.
func TestStdCopy(t *testing.T) {
	c, uri := open(t, testProg)
	c.diagnostics(uri)
	var loc *location
	c.call("textDocument/definition", docPos(uri, pos(t, testProg, "info(", 0)), &loc)
	if loc == nil {
		t.Fatal("no definition in std")
	}
	src, err := os.ReadFile(uriToPath(loc.URI))
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(uriToPath(loc.URI)); err != nil || fi.Mode().Perm()&0o222 != 0 {
		t.Errorf("std copy should be read only: %v %v", fi.Mode(), err)
	}
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": loc.URI, "languageId": "xo", "version": 1, "text": string(src)}})
	for range 2 {
		// one publish for each open document, in either order
		var p publishDiagnosticsParams
		select {
		case m := <-c.notes:
			json.Unmarshal(m.Params, &p)
		case <-time.After(10 * time.Second):
			t.Fatal("no diagnostics")
		}
		if len(p.Diagnostics) != 0 {
			t.Errorf("%s: %+v", p.URI, p.Diagnostics)
		}
	}
	var h hover
	c.call("textDocument/hover", docPos(loc.URI, loc.Range.Start), &h)
	if !strings.Contains(h.Contents.Value, "info") {
		t.Errorf("hover in the std copy: %q", h.Contents.Value)
	}
	var edits []textEdit
	c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": loc.URI}, "options": map[string]any{}}, &edits)
	if len(edits) != 0 {
		t.Errorf("formatting a std copy: %+v", edits)
	}
	// a second definition rewrites the read only copy without failing
	os.Chmod(uriToPath(loc.URI), 0o644)
	os.WriteFile(uriToPath(loc.URI), []byte("stale"), 0o644)
	var loc2 *location
	c.call("textDocument/definition", docPos(uri, pos(t, testProg, "info(", 0)), &loc2)
	if got, _ := os.ReadFile(uriToPath(loc.URI)); loc2 == nil || string(got) != string(src) {
		t.Errorf("std copy not rewritten: %v", loc2)
	}
}
