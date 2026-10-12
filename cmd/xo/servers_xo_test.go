package main

// The server commands in Xo against the Go ones (decision 0178 stage 5:
// selfhost/cli/servers). With XO_CLI naming the self hosted binary, each
// test drives the Go command in process and the Xo binary as a process
// with the same requests and compares the responses byte for byte. The
// transcripts that run other xo commands (MCP tools, `xo serve` runs,
// `xo replay`'s build) match once those commands are ported to the same
// binary; the protocol transcripts match now.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/serve"
)

// xoCLI is the self hosted binary under test, or the test is skipped.
func xoCLI(t *testing.T) string {
	t.Helper()
	p := os.Getenv("XO_CLI")
	if p == "" {
		t.Skip("XO_CLI is not set (the self hosted xo to compare)")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// xoCLIEnv is the environment of the self hosted binary: the std sources
// it checks against (the Go binary embeds them) and no forwarding to a
// running `xo serve`.
func xoCLIEnv() []string {
	std, _ := filepath.Abs(filepath.Join("..", "..", "std"))
	return append(os.Environ(), "XO_STD="+std, serve.DisableEnv+"=1")
}

// TestXo1ServersVersion: the version the Xo lsp reports is cmd/xo's.
func TestXo1ServersVersion(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "selfhost", "cli", "servers", "servers.xo"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`pub const XO_VERSION = "([^"]*)"`).FindSubmatch(src)
	if m == nil || string(m[1]) != version {
		t.Fatalf("selfhost/cli/servers XO_VERSION = %q, cmd/xo version = %q", m, version)
	}
}

// mcpProtocolTranscript is a session of requests answered without running
// any other command: every line's response must be the Go server's.
const mcpProtocolTranscript = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"ping"}
{"jsonrpc":"2.0","id":3,"method":"tools/list"}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}
{"jsonrpc":"2.0","id":5,"method":"tools/call"}
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":[1]}
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":5}}
{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"check","arguments":[1]}}
{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"check","arguments":{"dirs":5}}}
{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"check","arguments":{"dirs":["x",1]}}}
{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"check","arguments":{"no_gc":"yes"}}}
{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"query","arguments":{"kind":5}}}
{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"test","arguments":{"seed":"x"}}}
{"jsonrpc":"2.0","id":14,"method":"no/such"}
{"jsonrpc":"1.0","id":15,"method":"ping"}
{"method":"ping"}
{bad
[1]
{"jsonrpc":"2.0","id":16,"method":5}
{"jsonrpc":"2.0","id":null,"method":"ping"}
{ "jsonrpc" : "2.0" , "id" : "a b" , "method" : "ping" }
{"jsonrpc":"2.0","id":17,"method":"notifications/cancelled"}
{"JSONRPC":"2.0","ID":18,"Method":"ping"}
{"jsonrpc":"2.0","id":19,"method":"<x>&"}
{"jsonrpc":"2.0","id":20,"method":"é\u2028"}


null
"str"
{"jsonrpc":"2.0","id":21.50,"method":"ping"}
{"jsonrpc":"2.0","id":{"a":[1, 2]},"method":"ping"}
{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"build","arguments":{"dirs":[],"targets":7}}}
{"jsonrpc":"2.0","id":23,"method":"tools/call","params":{"name":"edit","arguments":{"op":"rename","args":"x"}}}
{"jsonrpc":"2.0","id":24,"method":"tools/call","params":{"name":"fmt","arguments":{"write":1}}}
{"jsonrpc":"2.0","id":26,"method":"tools/call","params":{"name":"check","arguments":{"dirs":["a"],"dirs":7}}}
{"jsonrpc":"2.0","id":27,"method":"initialize"}
{"jsonrpc":"2.0","id":28,"method":"tools/call","params":{"name":"query","arguments":{"Kind":"x","kind":null,"arg":3}}}
{"jsonrpc":"2.0","id":29,"method":"tools/call","params":{"name":"test","arguments":{"dirs":["d"],"run":7}}}
{"jsonrpc":"2.0","id":30,"method":"tools/call","params":{"name":"build","arguments":{"out":false}}}
{"jsonrpc":"2.0","id":31,"method":"tools/call","params":{"name":"explain","arguments":{"code":["XO0201"]}}}
`

// compareLines reports the lines of two outputs that differ.
func compareLines(t *testing.T, what string, goOut, xoOut string) (same, total int) {
	t.Helper()
	g := strings.Split(strings.TrimRight(goOut, "\n"), "\n")
	x := strings.Split(strings.TrimRight(xoOut, "\n"), "\n")
	total = max(len(g), len(x))
	for i := 0; i < total; i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(x) {
			b = x[i]
		}
		if a == b {
			same++
			continue
		}
		t.Errorf("%s line %d differs:\n go: %s\n xo: %s", what, i+1, a, b)
	}
	return same, total
}

// runXo runs the self hosted binary with stdin and returns its output.
func runXo(t *testing.T, bin, dir string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = xoCLIEnv()
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", bin, args, err)
	}
	return out.String(), errOut.String(), code
}

func TestXoMCPProtocol(t *testing.T) {
	bin := xoCLI(t)
	var goOut, goErr bytes.Buffer
	gcode := runMCP(nil, strings.NewReader(mcpProtocolTranscript), &goOut, &goErr)
	xoOut, xoErr, xcode := runXo(t, bin, t.TempDir(), mcpProtocolTranscript, "mcp")
	if gcode != xcode {
		t.Errorf("exit %d (go) vs %d (xo); stderr %q vs %q", gcode, xcode, goErr.String(), xoErr)
	}
	same, total := compareLines(t, "mcp", goOut.String(), xoOut)
	t.Logf("mcp protocol transcript: %d of %d responses identical", same, total)
	// flags: an unknown one is a usage error with the same text
	var e bytes.Buffer
	gc := runMCP([]string{"--nope"}, strings.NewReader(""), io.Discard, &e)
	_, xe, xc := runXo(t, bin, t.TempDir(), "", "mcp", "--nope")
	if gc != xc || e.String() != xe {
		t.Errorf("mcp --nope: %d %q (go) vs %d %q (xo)", gc, e.String(), xc, xe)
	}
}

// TestXoMCPTools runs every tool on a module with a known error: each
// runs an xo command line, so it matches once the command is ported.
func TestXoMCPTools(t *testing.T) {
	bin := xoCLI(t)
	dir := writeFiles(t, map[string]string{
		"app/main.xo":    knownErrorModule,
		"m/m.xo":         "fn add(a: Int, b: Int) -> Int { a + b }\n",
		"m/m_test.xo":    "test \"adds\" {\n  expect(add(1, 2)) == 3\n}\n",
		"fmtme/bad.xo":   "fn f()->Int{1}\n",
		"ok/main.xo":     "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"ok/xo.mod":      "module example.com/ok\nedition preview\n",
		"app/xo.mod":     "module example.com/app\nedition preview\n",
		"fmtme/xo.mod":   "module example.com/fmtme\nedition preview\n",
		"m/xo.mod":       "module example.com/m\nedition preview\n",
		"other/note.txt": "not xo\n",
	})
	app := filepath.Join(dir, "app")
	calls := []string{
		`{"name":"check","arguments":{"dirs":["` + app + `"]}}`,
		`{"name":"check","arguments":{"dirs":["` + filepath.Join(dir, "ok") + `"],"no_gc":true}}`,
		`{"name":"query","arguments":{"kind":"def","arg":"g","dir":"` + app + `"}}`,
		`{"name":"query","arguments":{"kind":"def","arg":"nope","dir":"` + app + `"}}`,
		`{"name":"edit","arguments":{"op":"rename","args":["g","h"],"dir":"` + app + `","dry_run":true}}`,
		`{"name":"explain","arguments":{"code":"XO0201"}}`,
		`{"name":"explain","arguments":{"code":"XO9999"}}`,
		`{"name":"fmt","arguments":{"paths":["` + filepath.Join(dir, "fmtme") + `"]}}`,
		`{"name":"test","arguments":{"dirs":["` + filepath.Join(dir, "m") + `"]}}`,
		`{"name":"build","arguments":{"dirs":["` + filepath.Join(dir, "ok") + `"],"out":"` + filepath.Join(dir, "ok.bin") + `"}}`,
	}
	var in strings.Builder
	for i, c := range calls {
		fmt.Fprintf(&in, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`+"\n", i+1, c)
	}
	var goOut bytes.Buffer
	runMCP(nil, strings.NewReader(in.String()), &goOut, io.Discard)
	xoOut, _, _ := runXo(t, bin, dir, in.String(), "mcp")
	same, total := compareLines(t, "mcp tools", goOut.String(), xoOut)
	t.Logf("mcp tool calls: %d of %d responses identical", same, total)
}

func TestXoDump(t *testing.T) {
	bin := xoCLI(t)
	if testing.Short() {
		t.Skip("builds a program")
	}
	dir := writeFiles(t, map[string]string{"s/main.xo": "fn main(os: Os) {\n  scope s {\n    s.spawn(fn() { os.clock.sleep(30s) })\n    os.clock.sleep(30s)\n  }\n}\n"})
	exe := filepath.Join(dir, "sleeper")
	if code := run([]string{"build", "-o", exe, filepath.Join(dir, "s")}, io.Discard, os.Stderr); code != 0 {
		t.Fatalf("build: %d", code)
	}
	p := exec.Command(exe)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		p.Process.Kill() // the process this test started
		p.Wait()
	}()
	time.Sleep(500 * time.Millisecond)
	pid := strconv.Itoa(p.Process.Pid)
	cases := [][]string{{pid}, {"abc"}, {"0"}, {}, {"1", "2"}, {"--timeout=x", "5"}, {"--nope", "5"}, {"-h"}, {"2147483000"}}
	same := 0
	for _, args := range cases {
		var gout, gerr bytes.Buffer
		gc := run(append([]string{"dump"}, args...), &gout, &gerr)
		xout, xerr, xc := runXo(t, bin, dir, "", append([]string{"dump"}, args...)...)
		if gc != xc || gout.String() != xout || gerr.String() != xerr {
			t.Errorf("dump %v:\n go %d %q %q\n xo %d %q %q", args, gc, gout.String(), gerr.String(), xc, xout, xerr)
			continue
		}
		same++
	}
	t.Logf("dump: %d of %d cases identical", same, len(cases))
}

// serveExchange sends raw request bytes to a server and returns the
// response line.
func serveExchange(t *testing.T, sock, req string) string {
	t.Helper()
	c, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", sock, err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(60 * time.Second))
	io.WriteString(c, req)
	if !strings.HasSuffix(req, "\n") {
		c.(*net.UnixConn).CloseWrite()
	}
	line, _ := bufio.NewReader(c).ReadString('\n')
	return line
}

// startXoServe starts `$XO_CLI serve --socket=sock` and waits until it
// answers.
func startXoServe(t *testing.T, bin, sock string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin, "serve", "--socket="+sock)
	cmd.Env = xoCLIEnv()
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if _, err := serve.Call(sock, serve.Request{Op: "ping"}); err == nil {
			return cmd
		}
		time.Sleep(50 * time.Millisecond)
	}
	cmd.Process.Kill() // the process this test started
	cmd.Wait()
	t.Fatalf("xo serve did not answer: %s", errOut.String())
	return nil
}

func TestXoServeProtocol(t *testing.T) {
	bin := xoCLI(t)
	sockDir, err := os.MkdirTemp("", "xs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sockDir)
	goSock, xoSock := filepath.Join(sockDir, "go.sock"), filepath.Join(sockDir, "xo.sock")
	ln, err := serve.Listen(goSock)
	if err != nil {
		t.Fatal(err)
	}
	srv := serve.NewServer(serveRun)
	stopped := make(chan error, 1)
	go func() { stopped <- srv.Serve(ln) }()
	xs := startXoServe(t, bin, xoSock)
	defer func() {
		if xs.ProcessState == nil {
			xs.Process.Kill() // the process this test started
			xs.Wait()
		}
	}()
	missing := filepath.Join(sockDir, "missing")
	reqs := []string{
		`{"op":"ping"}` + "\n",
		`{"op":"stats"}` + "\n",
		`{"op":"nope"}` + "\n",
		`{bad` + "\n",
		``,
		`{"op":"run"}` + "\n",
		`{"op":"run","args":["build","."]}` + "\n",
		`{"op":"run","args":["check","--json","."],"cwd":"` + missing + `"}` + "\n",
		`{"op":"ping"}`,
		`{"op":"stats"}` + "\n",
	}
	same := 0
	for _, r := range reqs {
		g := serveExchange(t, goSock, r)
		x := serveExchange(t, xoSock, r)
		if g != x {
			t.Errorf("request %q:\n go: %s\n xo: %s", r, g, x)
			continue
		}
		same++
	}
	t.Logf("serve protocol: %d of %d responses identical", same, len(reqs))
	// each CLI's status and stop against the other's server
	var out, errOut bytes.Buffer
	if code := run([]string{"serve", "--socket=" + xoSock, "status"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), `"hits": 0`) {
		t.Errorf("go status of the xo server: %d %s %s", code, out.String(), errOut.String())
	}
	xout, xerr, xc := runXo(t, bin, sockDir, "", "serve", "--socket="+goSock, "status")
	if xc != 0 || xout != out.String() {
		t.Errorf("xo status of the go server: %d %q %q, want %q", xc, xout, xerr, out.String())
	}
	if _, _, xc := runXo(t, bin, sockDir, "", "serve", "--socket="+goSock, "stop"); xc != 0 {
		t.Errorf("xo stop of the go server: %d", xc)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("go serve: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("the go server did not stop")
	}
	if code := run([]string{"serve", "--socket=" + xoSock, "stop"}, io.Discard, io.Discard); code != 0 {
		t.Errorf("go stop of the xo server: %d", code)
	}
	done := make(chan error, 1)
	go func() { done <- xs.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("xo serve exit: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("the xo server did not stop")
	}
	if _, err := os.Stat(xoSock); err == nil {
		t.Error("the xo server left its socket file")
	}
	// no server: both CLIs say so the same way
	var e1 bytes.Buffer
	g1 := run([]string{"serve", "--socket=" + xoSock, "status"}, io.Discard, &e1)
	_, xe1, x1 := runXo(t, bin, sockDir, "", "serve", "--socket="+xoSock, "status")
	if g1 != x1 || e1.String() != xe1 {
		t.Errorf("status without a server: %d %q (go) vs %d %q (xo)", g1, e1.String(), x1, xe1)
	}
	var e2 bytes.Buffer
	g2 := run([]string{"serve", "bogus"}, io.Discard, &e2)
	_, xe2, x2 := runXo(t, bin, sockDir, "", "serve", "--socket="+xoSock, "bogus")
	if g2 != x2 || e2.String() != xe2 {
		t.Errorf("serve bogus: %d %q (go) vs %d %q (xo)", g2, e2.String(), x2, xe2)
	}
}

// TestXoServeCache runs check and query through the Xo server: the
// responses and the hit and miss counts must be the Go server's.
func TestXoServeCache(t *testing.T) {
	bin := xoCLI(t)
	sockDir, err := os.MkdirTemp("", "xs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sockDir)
	goSock, xoSock := filepath.Join(sockDir, "go.sock"), filepath.Join(sockDir, "xo.sock")
	ln, err := serve.Listen(goSock)
	if err != nil {
		t.Fatal(err)
	}
	srv := serve.NewServer(serveRun)
	go srv.Serve(ln)
	defer serve.Call(goSock, serve.Request{Op: "shutdown"})
	xs := startXoServe(t, bin, xoSock)
	defer func() {
		serve.Call(xoSock, serve.Request{Op: "shutdown"})
		xs.Wait()
	}()
	dir := writeFiles(t, map[string]string{"app/main.xo": knownErrorModule})
	app := filepath.Join(dir, "app")
	step := func(what string, r serve.Request) {
		t.Helper()
		g, gerr := serve.Call(goSock, r)
		x, xerr := serve.Call(xoSock, r)
		if gerr != nil || xerr != nil {
			t.Fatalf("%s: %v %v", what, gerr, xerr)
		}
		if fmt.Sprint(*g) != fmt.Sprint(*x) {
			gj, _ := json.Marshal(g)
			xj, _ := json.Marshal(x)
			t.Errorf("%s:\n go: %s\n xo: %s", what, gj, xj)
		}
	}
	step("first check", serve.Request{Op: "run", Args: []string{"check", "--json", app}, Cwd: dir})
	step("stats", serve.Request{Op: "stats"})
	step("second check", serve.Request{Op: "run", Args: []string{"check", "--json", app}, Cwd: dir})
	step("stats", serve.Request{Op: "stats"})
	src := strings.Replace(knownErrorModule, "  g()\n}", "  g()\n  g()\n}", 1)
	if err := os.WriteFile(filepath.Join(app, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	step("check after an edit", serve.Request{Op: "run", Args: []string{"check", "--json", app}, Cwd: dir})
	step("query", serve.Request{Op: "run", Args: []string{"query", "def", "--json", "--dir=" + app, "g"}, Cwd: dir})
	step("stats", serve.Request{Op: "stats"})
}

// lspPeer is one LSP server under test: its input and its framed output.
type lspPeer struct {
	in   io.WriteCloser
	out  *bufio.Reader
	msgs chan string
}

func newLSPPeer(in io.WriteCloser, out io.Reader) *lspPeer {
	p := &lspPeer{in: in, out: bufio.NewReader(out), msgs: make(chan string, 100)}
	go func() {
		defer close(p.msgs)
		for {
			n := -1
			for {
				line, err := p.out.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
					n, _ = strconv.Atoi(v)
				}
			}
			body := make([]byte, max(n, 0))
			if _, err := io.ReadFull(p.out, body); err != nil {
				return
			}
			p.msgs <- string(body)
		}
	}()
	return p
}

func (p *lspPeer) send(body string) {
	io.WriteString(p.in, "Content-Length: "+strconv.Itoa(len(body))+"\r\n\r\n"+body)
}

// until reads messages until one contains want, and returns them all.
func (p *lspPeer) until(t *testing.T, want string) []string {
	t.Helper()
	var got []string
	for {
		select {
		case m, ok := <-p.msgs:
			if !ok {
				t.Errorf("the server ended waiting for %s (got %v)", want, got)
				return got
			}
			got = append(got, m)
			if strings.Contains(m, want) {
				return got
			}
		case <-time.After(30 * time.Second):
			t.Errorf("no message with %s (got %v)", want, got)
			return got
		}
	}
}

func TestXoLSPTranscript(t *testing.T) {
	bin := xoCLI(t)
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module example.com/app\nedition preview\n",
		"app/main.xo": knownErrorModule,
	})
	path := filepath.Join(dir, "app", "main.xo")
	uri := "file://" + path
	// the Go server in process
	gInR, gInW := io.Pipe()
	gOutR, gOutW := io.Pipe()
	gDone := make(chan int, 1)
	go func() { gDone <- runLSP(nil, gInR, gOutW, io.Discard); gOutW.Close() }()
	g := newLSPPeer(gInW, gOutR)
	// the Xo server
	cmd := exec.Command(bin, "lsp", "--stdio")
	cmd.Env = xoCLIEnv()
	xin, _ := cmd.StdinPipe()
	xout, _ := cmd.StdoutPipe()
	var xerr bytes.Buffer
	cmd.Stderr = &xerr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill() // the process this test started
			cmd.Wait()
		}
	}()
	x := newLSPPeer(xin, xout)
	q := strconv.Quote
	fixed := strings.Replace(knownErrorModule, "  g()\n}", "  g()?\n}", 1)
	fixed = strings.Replace(fixed, "fn main(os: Os) {", "fn main(os: Os) ! {", 1)
	steps := []struct{ send, until string }{
		{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`, `"id":1`},
		{`{"jsonrpc":"2.0","method":"initialized","params":{}}`, ""},
		{`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":` + q(uri) + `,"languageId":"xo","version":1,"text":` + q(knownErrorModule) + `}}}`, "publishDiagnostics"},
		{`{"jsonrpc":"2.0","id":2,"method":"textDocument/codeAction","params":{"textDocument":{"uri":` + q(uri) + `},"range":{"start":{"line":0,"character":0},"end":{"line":9,"character":0}},"context":{"diagnostics":[]}}}`, `"id":2`},
		{`{"jsonrpc":"2.0","id":3,"method":"textDocument/documentSymbol","params":{"textDocument":{"uri":` + q(uri) + `}}}`, `"id":3`},
		{`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":` + q(uri) + `,"version":2},"contentChanges":[{"text":` + q(fixed) + `}]}}`, "publishDiagnostics"},
		{`{"jsonrpc":"2.0","id":4,"method":"textDocument/documentSymbol","params":{"textDocument":{"uri":` + q(uri) + `}}}`, `"id":4`},
		{`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":` + q(uri) + `,"version":3},"contentChanges":[{"range":{"start":{"line":3,"character":2},"end":{"line":3,"character":3}},"text":"zz"}]}}`, "publishDiagnostics"},
		{`{"jsonrpc":"2.0","id":5,"method":"no/such"}`, `"id":5`},
		{`{"jsonrpc":"2.0","id":6,"method":"textDocument/hover","params":{"textDocument":{"uri":"file:///nowhere/x.xo"},"position":{"line":0,"character":0}}}`, `"id":6`},
		{`{"jsonrpc":"2.0","id":7,"method":"textDocument/semanticTokens/full","params":{"textDocument":{"uri":"file:///nowhere/x.xo"}}}`, `"id":7`},
		{`{"jsonrpc":"2.0","id":8,"method":"textDocument/formatting","params":{"textDocument":{"uri":"file:///nowhere/x.xo"}}}`, `"id":8`},
		{`{"jsonrpc":"2.0","method":"textDocument/didClose","params":{"textDocument":{"uri":` + q(uri) + `}}}`, "publishDiagnostics"},
		{`{"jsonrpc":"2.0","id":9,"method":"shutdown"}`, `"id":9`},
	}
	same := 0
	for i, s := range steps {
		g.send(s.send)
		x.send(s.send)
		if s.until == "" {
			same++
			continue
		}
		gm := g.until(t, s.until)
		xm := x.until(t, s.until)
		// the transcript exercises what it means to: the error, its fix,
		// the symbols
		if want := map[int]string{3: `"code":"XO0201"`, 4: `"kind":"quickfix"`, 5: `"name":"main"`}[i+1]; want != "" && !strings.Contains(strings.Join(gm, "\n"), want) {
			t.Errorf("step %d: the go server's answer lacks %s: %v", i+1, want, gm)
		}
		if strings.Join(gm, "\n") != strings.Join(xm, "\n") {
			t.Errorf("step %d (%s):\n go: %s\n xo: %s", i+1, s.send, strings.Join(gm, "\n     "), strings.Join(xm, "\n     "))
			continue
		}
		same++
	}
	g.send(`{"jsonrpc":"2.0","method":"exit"}`)
	x.send(`{"jsonrpc":"2.0","method":"exit"}`)
	if code := <-gDone; code != 0 {
		t.Errorf("go lsp exit %d", code)
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("xo lsp: %v %s", err, xerr.String())
	}
	t.Logf("lsp transcript: %d of %d steps identical", same, len(steps))
}

func TestXoReplay(t *testing.T) {
	bin := xoCLI(t)
	if testing.Short() {
		t.Skip("builds Go")
	}
	dir := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	src := filepath.Join(dir, "app")
	os.MkdirAll(src, 0o755)
	prog := "fn main(os: Os) {\n  os.stdio.println(\"args ${os.env.args().join(\",\")} rand ${os.rand.int(0, 1000000)} now ${os.clock.now()}\")\n}\n"
	os.WriteFile(filepath.Join(src, "main.xo"), []byte(prog), 0o644)
	trace := filepath.Join(dir, "run.xotrace")
	var out bytes.Buffer
	if code := run([]string{"run", "--record=" + trace, src, "--", "a", "b"}, &out, io.Discard); code != 0 {
		t.Fatalf("run --record: %d", code)
	}
	var gout, gerr bytes.Buffer
	gc := run([]string{"replay", trace}, &gout, &gerr)
	xout, xerr, xc := runXo(t, bin, dir, "", "replay", trace)
	if gc != xc || gout.String() != xout {
		t.Errorf("replay:\n go %d %q %q\n xo %d %q %q", gc, gout.String(), gerr.String(), xc, xout, xerr)
	}
	if gout.String() != out.String() {
		t.Errorf("go replay %q, recorded %q", gout.String(), out.String())
	}
	// not a trace, and a usage error
	for _, args := range [][]string{{"replay", filepath.Join(src, "main.xo")}, {"replay"}, {"replay", "a", "b"}, {"replay", "--nope"}} {
		var e bytes.Buffer
		g := run(args, io.Discard, &e)
		_, xe, x := runXo(t, bin, dir, "", args...)
		if g != x || e.String() != xe {
			t.Errorf("%v: %d %q (go) vs %d %q (xo)", args, g, e.String(), x, xe)
		}
	}
}
