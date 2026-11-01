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

// TestWasmComponentServe builds testdata/wasm_http_serve with `xo build
// --backend=wasm`, checks the component Xo wrote with `wasm-tools validate`
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
	build := testtmp.Guard(exec.Command(xo, "build", "--backend=wasm", "-o", comp, "."))
	build.Dir = dir
	build.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=wasm: %v\n%s", err, b)
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
	req, _ := http.NewRequest("GET", base+"/fault", nil)
	if resp, err := client.Do(req); err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 500 {
			t.Errorf("GET /fault: status %d, want an error", resp.StatusCode)
		}
	}
	if status, got := do("GET", "/", "", nil); status != 200 || got != "hello" {
		t.Errorf("after the fault: %d %q", status, got)
	}
	time.Sleep(100 * time.Millisecond)
	if s := logs.String(); !strings.Contains(s, "to stdout yyy") || !strings.Contains(s, "to stderr") {
		t.Errorf("the host's output lacks the program's lines:\n%.2000s", s)
	}
}
