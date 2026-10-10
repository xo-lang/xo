package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/serve"
)

// TestMain keeps every test in process: a developer's running `xo serve`
// (built from other code) must not answer the tests' check and query runs.
// The serve test turns forwarding back on for its own temporary socket.
func TestMain(m *testing.M) {
	os.Setenv(serve.DisableEnv, "1")
	// identical builds reuse one binary, within and across runs
	build.EnableTestBinCache()
	os.Exit(m.Run())
}

const knownErrorModule = "enum E { A }\nfn g() -> Int ! E { 1 }\nfn main(os: Os) {\n  g()\n}\n"

// mcpClient drives `xo mcp` through pipes.
type mcpClient struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Reader
	id  int
}

func (c *mcpClient) send(method string, params any, notify bool) {
	c.t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	if !notify {
		c.id++
		msg["id"] = c.id
	}
	data, _ := json.Marshal(msg)
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

func (c *mcpClient) call(method string, params any) map[string]any {
	c.t.Helper()
	c.send(method, params, false)
	line, err := c.out.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("%s: %v", method, err)
	}
	var resp struct {
		ID     int            `json:"id"`
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		c.t.Fatalf("%s: bad response %s: %v", method, line, err)
	}
	if resp.ID != c.id || resp.Error != nil {
		c.t.Fatalf("%s: response %s", method, line)
	}
	return resp.Result
}

// tool calls a tool and decodes its JSON text content.
func (c *mcpClient) tool(name string, args map[string]any) (map[string]any, bool) {
	c.t.Helper()
	r := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	content := r["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if strings.Contains(text, "\n") {
		c.t.Errorf("%s: tool output is not compact JSON: %q", name, text)
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		c.t.Fatalf("%s: tool text is not JSON: %v\n%s", name, err, text)
	}
	m, _ := v.(map[string]any)
	if m == nil {
		m = map[string]any{"value": v}
	}
	return m, r["isError"] == true
}

func TestMCPSession(t *testing.T) {
	dir := writeFiles(t, map[string]string{"app/main.xo": knownErrorModule})
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	var errOut bytes.Buffer
	done := make(chan int)
	go func() {
		code := runMCP(nil, inR, outW, &errOut)
		outW.Close()
		done <- code
	}()
	c := &mcpClient{t: t, in: inW, out: bufio.NewReader(outR)}

	init := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	if init["protocolVersion"] != "2025-06-18" || init["serverInfo"].(map[string]any)["name"] != "xo" {
		t.Errorf("initialize = %v", init)
	}
	c.send("notifications/initialized", nil, true)

	list := c.call("tools/list", nil)["tools"].([]any)
	names := map[string]bool{}
	for _, tl := range list {
		m := tl.(map[string]any)
		names[m["name"].(string)] = true
		if m["inputSchema"].(map[string]any)["type"] != "object" {
			t.Errorf("tool %v has no object input schema", m["name"])
		}
	}
	for _, n := range []string{"check", "query", "edit", "explain", "fmt", "test", "build"} {
		if !names[n] {
			t.Errorf("tools/list lacks %s: %v", n, names)
		}
	}

	chk, isErr := c.tool("check", map[string]any{"dirs": []string{dir}})
	diags, _ := chk["diagnostics"].([]any)
	if isErr || chk["ok"] != false || len(diags) != 1 {
		t.Fatalf("check = %v", chk)
	}
	d := diags[0].(map[string]any)
	if d["code"] != "XO0201" || len(d["fixes"].([]any)) == 0 {
		t.Errorf("diagnostic = %v", d)
	}

	def, isErr := c.tool("query", map[string]any{"kind": "def", "arg": "g", "dir": dir})
	if isErr || !strings.Contains(fmt.Sprint(def), "fn g() -> Int ! E") {
		t.Errorf("query def = %v", def)
	}

	// The module has an error, so the edit is rejected with it.
	ed, isErr := c.tool("edit", map[string]any{"op": "rename", "args": []string{"g", "h"}, "dir": dir, "dry_run": true})
	if isErr || ed["ok"] != false || !strings.Contains(fmt.Sprint(ed["reason"]), "before the edit") {
		t.Errorf("edit = %v", ed)
	}

	ex, isErr := c.tool("explain", map[string]any{"code": "XO0201"})
	if isErr || ex["code"] != "XO0201" {
		t.Errorf("explain = %v", ex)
	}

	f, isErr := c.tool("fmt", map[string]any{"paths": []string{dir}})
	if isErr || f["written"] != false {
		t.Errorf("fmt = %v", f)
	}

	if bad, isErr := c.tool("query", map[string]any{"kind": "def", "arg": "nope", "dir": dir}); !isErr && bad["error"] == nil {
		t.Logf("query of a missing name = %v", bad)
	}

	// check with fix=true applies the machine fixes (decision 0096)
	fixDir := writeFiles(t, map[string]string{"app/main.xo": "fn f(n: Int) -> Int {\n  if n > 0 {\n    return 1\n  }\n  else {\n    return 2\n  }\n}\nfn main(os: Os) {\n  os.stdio.println(\"${f(1)}\")\n}\n"})
	fx, isErr := c.tool("check", map[string]any{"dirs": []string{fixDir}, "fix": true})
	if applied, _ := fx["applied"].([]any); isErr || fx["ok"] != true || len(applied) != 1 {
		t.Errorf("check fix = %v", fx)
	}
	if _, has := chk["applied"]; has {
		t.Errorf("plain check has applied: %v", chk)
	}

	// an unknown method is a JSON-RPC error, not a crash
	c.send("no/such", nil, false)
	line, _ := c.out.ReadBytes('\n')
	if !strings.Contains(string(line), "-32601") {
		t.Errorf("unknown method = %s", line)
	}

	inW.Close()
	if code := <-done; code != 0 {
		t.Errorf("xo mcp exit %d: %s", code, errOut.String())
	}
}

func TestMCPTestTool(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a Go binary")
	}
	dir := writeFiles(t, map[string]string{
		"m/m.xo":      "fn add(a: Int, b: Int) -> Int { a + b }\n",
		"m/m_test.xo": "test \"adds\" {\n  expect(add(1, 2)) == 3\n}\n\ntest \"wrong\" {\n  expect(add(1, 1)) == 3\n}\n",
	})
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{"dirs":["` + filepath.Join(dir, "m") + `"]}}}` + "\n")
	if code := runMCP(nil, in, &out, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil || len(resp.Result.Content) == 0 {
		t.Fatalf("bad response %s: %v", out.String(), err)
	}
	var r struct {
		OK       bool `json:"ok"`
		Passed   int  `json:"passed"`
		Failed   int  `json:"failed"`
		Failures []struct {
			Name string `json:"name"`
		} `json:"failures"`
	}
	if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &r); err != nil {
		t.Fatal(err)
	}
	if r.OK || r.Passed != 1 || r.Failed != 1 || len(r.Failures) != 1 || r.Failures[0].Name != "wrong" {
		t.Errorf("test tool = %s", resp.Result.Content[0].Text)
	}
}

func TestServe(t *testing.T) {
	sockDir, err := os.MkdirTemp("", "xs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sockDir)
	sock := filepath.Join(sockDir, "s.sock")
	ln, err := serve.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := serve.NewServer(serveRun)
	stopped := make(chan error, 1)
	go func() { stopped <- srv.Serve(ln) }()
	defer func() {
		if _, err := serve.Call(sock, serve.Request{Op: "shutdown"}); err != nil {
			t.Errorf("shutdown: %v", err)
		}
		select {
		case err := <-stopped:
			if err != nil {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("server did not stop")
		}
	}()
	t.Setenv(serve.SocketEnv, sock)
	t.Setenv(serve.DisableEnv, "")

	dir := writeFiles(t, map[string]string{"app/main.xo": knownErrorModule})
	app := filepath.Join(dir, "app")
	checkVia := func() (string, time.Duration) {
		t.Helper()
		var out, errOut bytes.Buffer
		start := time.Now()
		run([]string{"check", "--json", app}, &out, &errOut)
		return out.String(), time.Since(start)
	}
	stats := func() serve.Stats {
		t.Helper()
		r, err := serve.Call(sock, serve.Request{Op: "stats"})
		if err != nil || r.Stats == nil {
			t.Fatalf("stats: %v", err)
		}
		return *r.Stats
	}

	first, d1 := checkVia()
	if !strings.Contains(first, "XO0201") {
		t.Fatalf("first check = %s", first)
	}
	if s := stats(); s.Misses != 1 || s.Hits != 0 {
		t.Errorf("after first check: %+v (the check was not served)", s)
	}
	second, d2 := checkVia()
	if second != first {
		t.Errorf("second check differs:\n%s\n%s", first, second)
	}
	if s := stats(); s.Hits != 1 {
		t.Errorf("after second check: %+v, want a cache hit", s)
	}
	t.Logf("check: first %v, cached %v", d1, d2)

	r, err := serve.Call(sock, serve.Request{Op: "run", Args: []string{"check", "--json", app}})
	if err != nil || !r.Cached || r.Code != 1 {
		t.Errorf("direct run = %+v, %v", r, err)
	}

	// edit: a second unhandled call adds a diagnostic at line 5
	src := strings.Replace(knownErrorModule, "  g()\n}", "  g()\n  g()\n}", 1)
	if err := os.WriteFile(filepath.Join(app, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	third, _ := checkVia()
	if strings.Count(third, `"XO0201"`) != 2 || !strings.Contains(third, `"line": 5`) {
		t.Errorf("after edit = %s", third)
	}
	if s := stats(); s.Misses != 2 {
		t.Errorf("after edit: %+v, want a recheck", s)
	}

	// query goes through the same cache
	var out, errOut bytes.Buffer
	if code := run([]string{"query", "def", "--json", "--dir=" + app, "g"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "fn g()") {
		t.Errorf("query = %d %s %s", code, out.String(), errOut.String())
	}
	if s := stats(); s.Hits < 2 {
		t.Errorf("query was not served from the cache: %+v", s)
	}
}
