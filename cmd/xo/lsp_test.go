package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// lspClient drives `xo lsp` over its standard input and output with LSP's
// Content-Length framing.
type lspClient struct {
	t      *testing.T
	in     io.WriteCloser
	msgs   chan map[string]any
	nextID int
}

func startLSP(t *testing.T, dir string) *lspClient {
	t.Helper()
	cmd := command(t, xoBin(t), "lsp")
	cmd.Dir = dir
	// xo reads its working directory from $PWD (decision 0218 replaces it)
	cmd.Env = append(cmd.Env, "PWD="+dir)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &lspClient{t: t, in: in, msgs: make(chan map[string]any, 64)}
	go func() {
		defer close(c.msgs)
		r := bufio.NewReader(out)
		for {
			n := -1
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Content-Length") {
					n, _ = strconv.Atoi(strings.TrimSpace(v))
				}
			}
			body := make([]byte, n)
			if _, err := io.ReadFull(r, body); err != nil {
				return
			}
			var m map[string]any
			if err := json.Unmarshal(body, &m); err != nil {
				return
			}
			c.msgs <- m
		}
	}()
	t.Cleanup(func() {
		c.request("shutdown", nil)
		c.notify("exit", nil)
		in.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			// our own child, by the PID we started
			_ = cmd.Process.Kill()
			<-done
		}
	})
	return c
}

func (c *lspClient) send(m map[string]any) {
	m["jsonrpc"] = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(b), b); err != nil {
		c.t.Fatal(err)
	}
}

func (c *lspClient) notify(method string, params any) {
	m := map[string]any{"method": method}
	if params != nil {
		m["params"] = params
	}
	c.send(m)
}

// request sends a request and returns its result, skipping the
// notifications in between.
func (c *lspClient) request(method string, params any) any {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	m := map[string]any{"id": id, "method": method}
	if params != nil {
		m["params"] = params
	}
	c.send(m)
	timeout := time.After(3 * time.Minute)
	for {
		select {
		case msg, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("%s: server closed its output", method)
			}
			if got, ok := msg["id"].(float64); ok && int(got) == id {
				if e, ok := msg["error"]; ok {
					c.t.Fatalf("%s: error %v", method, e)
				}
				return msg["result"]
			}
		case <-timeout:
			c.t.Fatalf("%s: no reply", method)
		}
	}
}

func lspPos(uri string, line, ch int) map[string]any {
	return map[string]any{"textDocument": map[string]any{"uri": uri}, "position": map[string]any{"line": line, "character": ch}}
}

// lspAt is the position of the end of the first occurrence of mark in
// src (0 based line and character).
func lspAt(t *testing.T, src, mark string) (int, int) {
	t.Helper()
	i := strings.Index(src, mark)
	if i < 0 {
		t.Fatalf("no %q in the source", mark)
	}
	i += len(mark)
	line := strings.Count(src[:i], "\n")
	return line, i - (strings.LastIndex(src[:i], "\n") + 1)
}

func completionLabels(t *testing.T, res any) map[string]map[string]any {
	t.Helper()
	obj, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("completion result = %v", res)
	}
	items, _ := obj["items"].([]any)
	out := map[string]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		out[m["label"].(string)] = m
	}
	return out
}

// TestLSPCompletion drives `xo lsp` (decisions 0114, 0219): completion of
// members, package members, and names in scope, signature help, and hover
// and definition into std.
func TestLSPCompletion(t *testing.T) {
	t.Parallel()
	src := `use std/http

struct Point {
  x: Int
  y: Int
}

fn area(width: Int, height: Int) -> Int {
  width * height
}

fn main(os: Os) {
  let p = Point{x: 1, y: 2}
  let r = http.new_request("GET", "/")
  let total: Int = p.x
  os.stdio.println("${p.y} ${r.path} ${total}")
  let a = area(1, 2)
  os.stdio.println("${a}")
}
`
	// the lines completion is asked at, each a separate edit of src
	dir := writeFiles(t, map[string]string{"xo.mod": "module example.com/lspc\n", "main.xo": src})
	path := filepath.Join(dir, "main.xo")
	uri := "file://" + path
	c := startLSP(t, dir)
	initRes, _ := c.request("initialize", map[string]any{"processId": nil, "rootUri": "file://" + dir, "capabilities": map[string]any{}}).(map[string]any)
	caps, _ := initRes["capabilities"].(map[string]any)
	if _, ok := caps["completionProvider"]; !ok {
		t.Errorf("no completionProvider in %v", caps)
	}
	if _, ok := caps["signatureHelpProvider"]; !ok {
		t.Errorf("no signatureHelpProvider in %v", caps)
	}
	c.notify("initialized", map[string]any{})
	version := 1
	open := func(text string) {
		if version == 1 {
			c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "xo", "version": version, "text": text}})
		} else {
			c.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": uri, "version": version}, "contentChanges": []any{map[string]any{"text": text}}})
		}
		version++
	}

	// hover and definition into std (regression checks)
	open(src)
	l, ch := lspAt(t, src, "http.new_req")
	hov, _ := c.request("textDocument/hover", lspPos(uri, l, ch)).(map[string]any)
	if s := fmt.Sprint(hov["contents"]); !strings.Contains(s, "new_request") {
		t.Errorf("hover on new_request = %v", hov)
	}
	l, ch = lspAt(t, src, "${r.pa")
	def, _ := c.request("textDocument/definition", lspPos(uri, l, ch)).(map[string]any)
	if s := fmt.Sprint(def["uri"]); !strings.HasSuffix(s, "/http.xo") {
		t.Errorf("definition of r.path = %v, want a location in std http.xo", def)
	}

	// members of a struct after `p.`, with the line after it intact
	edited := strings.Replace(src, "  let total: Int = p.x\n", "  let total: Int = p.\n", 1)
	open(edited)
	l, ch = lspAt(t, edited, "Int = p.")
	got := completionLabels(t, c.request("textDocument/completion", lspPos(uri, l, ch)))
	for _, want := range []string{"x", "y"} {
		if _, ok := got[want]; !ok {
			t.Errorf("completion after p. has no %q: %v", want, got)
		}
	}
	if x := got["x"]; x != nil && x["detail"] != "x: Int" {
		t.Errorf("detail of x = %v, want \"x: Int\"", x["detail"])
	}

	// package members after `http.`
	edited = strings.Replace(src, "  let total: Int = p.x\n", "  let q = http.\n  let total: Int = p.x\n", 1)
	open(edited)
	l, ch = lspAt(t, edited, "q = http.")
	got = completionLabels(t, c.request("textDocument/completion", lspPos(uri, l, ch)))
	if _, ok := got["new_request"]; !ok {
		t.Errorf("completion after http. has no new_request: %d items", len(got))
	}

	// the request's fields and methods after `r.`
	edited = strings.Replace(src, "  let total: Int = p.x\n", "  r.\n  let total: Int = p.x\n", 1)
	open(edited)
	l, ch = lspAt(t, edited, "  r.")
	got = completionLabels(t, c.request("textDocument/completion", lspPos(uri, l, ch)))
	for _, want := range []string{"method", "path", "headers"} {
		if _, ok := got[want]; !ok {
			t.Errorf("completion after r. has no %q: %d items", want, len(got))
		}
	}

	// a bare prefix: names in scope, module names, keywords
	edited = strings.Replace(src, "  let a = area(1, 2)\n", "  let a = ar\n", 1)
	open(edited)
	l, ch = lspAt(t, edited, "a = ar")
	got = completionLabels(t, c.request("textDocument/completion", lspPos(uri, l, ch)))
	if _, ok := got["area"]; !ok {
		t.Errorf("completion of ar has no area: %v", got)
	}
	edited = strings.Replace(src, "  let a = area(1, 2)\n", "  let a = \n", 1)
	open(edited)
	l, ch = lspAt(t, edited, "let a = ")
	got = completionLabels(t, c.request("textDocument/completion", lspPos(uri, l, ch)))
	for _, want := range []string{"p", "r", "total", "area", "Point", "http", "match", "Str"} {
		if _, ok := got[want]; !ok {
			t.Errorf("completion of an empty name has no %q: %d items", want, len(got))
		}
	}
	if _, ok := got["a"]; ok {
		t.Errorf("completion of an empty name offers `a`, declared after the cursor")
	}
	// the expected type sorts first: `let total: Int = ` ranks Int items
	edited = strings.Replace(src, "  let total: Int = p.x\n", "  let total: Int = \n", 1)
	open(edited)
	l, ch = lspAt(t, edited, "total: Int = ")
	got = completionLabels(t, c.request("textDocument/completion", lspPos(uri, l, ch)))
	if a := got["area"]; a == nil || !strings.HasPrefix(fmt.Sprint(a["sortText"]), "0") {
		t.Errorf("area (-> Int) does not sort first for an Int: %v", a)
	}
	if p := got["p"]; p == nil || strings.HasPrefix(fmt.Sprint(p["sortText"]), "0") {
		t.Errorf("p (a Point) sorts first for an Int: %v", p)
	}

	// signature help inside an open call, second argument
	edited = strings.Replace(src, "  let a = area(1, 2)\n", "  let a = area(1, \n", 1)
	open(edited)
	l, ch = lspAt(t, edited, "area(1, ")
	sh, _ := c.request("textDocument/signatureHelp", lspPos(uri, l, ch)).(map[string]any)
	sigs, _ := sh["signatures"].([]any)
	if len(sigs) != 1 {
		t.Fatalf("signatureHelp = %v", sh)
	}
	if lbl := sigs[0].(map[string]any)["label"]; lbl != "area(width: Int, height: Int) -> Int" {
		t.Errorf("signature label = %v", lbl)
	}
	if ap := sh["activeParameter"]; ap != float64(1) {
		t.Errorf("activeParameter = %v, want 1", ap)
	}
}
