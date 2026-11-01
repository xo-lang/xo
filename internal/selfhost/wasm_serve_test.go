package selfhost

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmComponentTargets checks what each WASI target gives (decision 0447):
// the output's kind, and XO1514 to XO1516 for a program that does not fit.
func TestWasmComponentTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	const serve = `use std/http

fn main(os: Os) {
  var r = http.router()
  r.handle("GET /", fn(q: http.Request) -> http.Response { %s http.text(200, "hello") })
  http.serve_component(r)
}
`
	plain := "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"
	component := []byte{0x00, 0x61, 0x73, 0x6d, 0x0d, 0x00, 0x01, 0x00}
	module := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, c := range []struct {
		name, src string
		args      []string
		want      []byte // the output's first bytes, or nil for a failed build
		code      string // in the output of a failed build
		also      string
	}{
		{"component", fmt.Sprintf(serve, ""), []string{"--targets=wasip2/wasm"}, component, "", ""},
		{"component with the backend named", fmt.Sprintf(serve, ""), []string{"--backend=wasm", "--targets=wasip2/wasm"}, component, "", ""},
		{"module", plain, []string{"--targets=wasip1/wasm"}, module, "", ""},
		{"module by backend", plain, []string{"--backend=wasm"}, module, "", ""},
		{"component on wasip1", fmt.Sprintf(serve, ""), []string{"--targets=wasip1/wasm"}, nil, "XO1514", "--targets=wasip2/wasm"},
		{"component by backend", fmt.Sprintf(serve, ""), []string{"--backend=wasm"}, nil, "XO1514", "--targets=wasip2/wasm"},
		{"no router", plain, []string{"--targets=wasip2/wasm"}, nil, "XO1516", "serve_component"},
		{"a file read", fmt.Sprintf(serve, `let _b = os.fs.read("/etc/hosts") ?? _ => b"";`), []string{"--targets=wasip2/wasm"}, nil, "XO1515", "files"},
		{"a process", fmt.Sprintf(serve, `let _p = os.proc.run("true", []).ignore_err("test");`), []string{"--targets=wasip2/wasm"}, nil, "XO1515", "processes"},
		{"a socket", fmt.Sprintf(serve, `let _l = os.net.listen("127.0.0.1:0").ignore_err("test");`), []string{"--targets=wasip2/wasm"}, nil, "XO1515", "sockets"},
		{"both wasi targets", fmt.Sprintf(serve, ""), []string{"--targets=wasip1/wasm,wasip2/wasm"}, nil, "", "name it alone"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(c.src), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module m\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "m.wasm")
			args := append(append([]string{"build"}, c.args...), "-o", out, ".")
			cmd := testtmp.Guard(exec.Command(xo, args...))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
			b, err := cmd.CombinedOutput()
			if c.want == nil {
				if err == nil {
					t.Fatalf("built, want a failure with %s %q", c.code, c.also)
				}
				if !strings.Contains(string(b), c.code) || !strings.Contains(string(b), c.also) {
					t.Fatalf("the output lacks %s %q:\n%s", c.code, c.also, b)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v\n%s", err, b)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(got, c.want) {
				t.Fatalf("output starts % x, want % x", got[:8], c.want)
			}
		})
	}
}

// TestWasmComponentServe builds testdata/wasm_http_serve with `xo build
// --targets=wasip2/wasm`, checks the component Xo wrote with `wasm-tools validate`
// and serves it with `wasmtime serve` given only the wasi:http/proxy world
// (decision 0447): both tools are test oracles, never build steps. It skips
// where they are not installed (the Mac); the linux/arm64 offload hosts have
// them.
func TestWasmComponentServe(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	for _, tool := range []string{"wasm-tools", "wasmtime"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "selfhost", "testdata", "wasm_http_serve")
	tmp := t.TempDir()
	comp := filepath.Join(tmp, "serve.wasm")
	build := testtmp.Guard(exec.Command(xo, "build", "--targets=wasip2/wasm", "-o", comp, "."))
	build.Dir = dir
	build.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("xo build --targets=wasip2/wasm: %v\n%s", err, b)
	}
	if b, err := testtmp.Guard(exec.Command("wasm-tools", "validate", comp)).CombinedOutput(); err != nil {
		t.Fatalf("wasm-tools validate: %v\n%s", err, b)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs bytes.Buffer
	srv := exec.CommandContext(ctx, "wasmtime", "serve", "--addr", addr, comp)
	srv.Stdout, srv.Stderr = &logs, &logs
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Wait()
	defer cancel()
	base := "http://" + addr
	client := &http.Client{Timeout: 30 * time.Second}
	up := time.Now().Add(30 * time.Second)
	for {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(up) {
			t.Fatalf("wasmtime serve did not listen\n%s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	do := func(method, path, body string, hdr map[string]string) (int, string) {
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", method, path, err, logs.String())
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("%s %s: reading the body: %v", method, path, err)
		}
		return resp.StatusCode, string(b)
	}
	for _, c := range []struct {
		method, path, body string
		hdr                map[string]string
		status             int
		want               string
	}{
		{"GET", "/", "", nil, 200, "hello"},
		{"GET", "/big", "", nil, 200, strings.Repeat("x", 20000)},
		{"POST", "/echo", strings.Repeat("z", 100000), nil, 200, "len 100000"},
		{"GET", "/head", "", map[string]string{"X-Test": "seen"}, 200, "seen"},
		{"GET", "/sleep", "", nil, 200, "slept true"},
		{"GET", "/now", "", nil, 200, "after 2020 true"},
		{"GET", "/rand", "", nil, 200, "rand 32"},
		{"GET", "/log", "", nil, 200, "logged"},
		// std's stdin reader is the native runtime's, which faults on wasm
		{"GET", "/stdin", "", nil, 500, `{"error":"internal error"}`},
		{"GET", "/env", "", nil, 200, "env - args 0"},
		{"GET", "/nope", "", nil, 404, `{"error":"not found"}`},
	} {
		status, got := do(c.method, c.path, c.body, c.hdr)
		if status != c.status || got != c.want {
			if len(got) > 200 {
				got = fmt.Sprintf("%s... (%d bytes)", got[:200], len(got))
			}
			t.Errorf("%s %s: %d %q, want %d %q", c.method, c.path, status, got, c.status, c.want)
		}
	}
	// a fault: the program's report on stderr, an error status or a dropped
	// connection, and the next request served by a new instance
	// and so for proc.exit, which the proxy world has no way to do
	for _, path := range []string{"/fault", "/exit"} {
		req, _ := http.NewRequest("GET", base+path, nil)
		if resp, err := client.Do(req); err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 {
				t.Errorf("GET %s: status %d, want an error", path, resp.StatusCode)
			}
		}
		if status, got := do("GET", "/", "", nil); status != 200 || got != "hello" {
			t.Errorf("after %s: %d %q", path, status, got)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if s := logs.String(); !strings.Contains(s, "to stdout yyy") || !strings.Contains(s, "to stderr") {
		t.Errorf("the host's output lacks the program's lines:\n%.2000s", s)
	}
}
