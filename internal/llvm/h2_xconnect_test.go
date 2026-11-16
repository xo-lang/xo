package llvm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestH2ExtConnect checks RFC 8441 extended CONNECT with it on
// (decision 0687 stage 1: H2Config{extended_connect: true}): the Go
// backend's server (net/http with GODEBUG=http2xconnect=1, the only way
// Go turns it on) and the native ones (std/native/llrt/h2srv.xo) must
// send the same SETTINGS entry and answer the same raw frames: an
// extended CONNECT reaches its handler with Request.protocol, one
// without :path is a stream error, and a :protocol on another method is
// admitted. Off (the default) is TestH2Diff's ":protocol" case.
func TestH2ExtConnect(t *testing.T) {
	xotool.NeedPorts(t)
	if testing.Short() {
		t.Skip("builds an HTTP/2 program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	pki := newTestPKI(t)
	dir := t.TempDir()
	prog := fmt.Sprintf(`use std/http

fn main(os: Os) {
  let cert = http.tls_cert(%q.bytes(), %q.bytes()).or_fault("cert")
  var r = http.router()
  r.handle("/ws", fn(req) { http.text(200, "protocol=${req.protocol()} method=${req.method} path=${req.path} scheme=${req.url.scheme} body=${req.body.len()} headers=${req.headers.len()}") })
  r.handle("GET /hello", fn(req) { http.text(200, "hello protocol=${req.protocol()}") })
  let l = os.net.listen("127.0.0.1:0").or_fault("listen")
  os.stdio.println("ADDR ${l.addr()}")
  match http.server(l, r).with_tls([cert]).with_h2_config(http.H2Config{extended_connect: true}).serve() {
    Ok(_) => os.stdio.println("served")
    Err(e) => os.stdio.println("serve: ${e}")
  }
}
`, pki.goodCert, pki.goodKey)
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	outs := make([]string, len(backends))
	t.Run("run", func(t *testing.T) {
		for i, be := range backends {
			i, be := i, be
			t.Run(be, func(t *testing.T) {
				t.Parallel()
				res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
				if err != nil {
					t.Fatalf("build (%s): %v", be, err)
				}
				if res.Failed {
					t.Fatalf("build (%s) failed: %v", be, res.Output)
				}
				outs[i] = runH2ExtConnect(t, res.Binaries[xotool.Host()], pki, be != xotool.BackendGo)
			})
		}
	})
	if t.Failed() {
		return
	}
	t.Logf("go backend:\n%s", outs[0])
	// the Go backend's answers, as RFC 8441 and Go's server give them
	for _, want := range []string{
		"settings: ENABLE_CONNECT_PROTOCOL=1",
		"extended connect: 200 protocol=websocket method=CONNECT path=/ws scheme=https body=0",
		"extended connect with data: 200 protocol=websocket method=CONNECT path=/ws scheme=https body=3 headers=1",
		"extended connect without :path: RST_STREAM PROTOCOL_ERROR",
		"get with :protocol: 200 hello protocol=websocket",
		"get without :protocol: 200 hello protocol=\n",
	} {
		if !strings.Contains(outs[0], want) {
			t.Errorf("go backend: no %q", want)
		}
	}
	for i, be := range backends[1:] {
		if got := outs[i+1]; got != outs[0] {
			t.Errorf("%s backend differs from the Go backend\n%s", be, lineDiff(outs[0], got))
		}
	}
}

// runH2ExtConnect runs the program, probes its server with raw frames,
// and returns what the probes saw.
func runH2ExtConnect(t *testing.T, bin string, p *testPKI, native bool) string {
	t.Helper()
	cmd := testtmp.Guard(exec.Command(bin))
	testtmp.Lifeline(t, cmd, 10*time.Minute)
	cmd.Env = append(os.Environ(), "GODEBUG=http2xconnect=1")
	if native {
		cmd.Env = append(cmd.Env, "XO_LEAKCHECK=1")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	rd := bufio.NewReader(stdout)
	addr := ""
	for addr == "" {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("program ended before serving: %v\nstderr:\n%s", err, stderr.String())
		}
		addr, _ = strings.CutPrefix(strings.TrimSpace(line), "ADDR ")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "settings: %s\n", h2ServerSettings(addr, p))
	connect := func(extra ...string) []string {
		return append([]string{":method", "CONNECT", ":protocol", "websocket"}, extra...)
	}
	cases := []struct {
		name string
		send func(h *h2conn)
	}{
		{"extended connect", func(h *h2conn) {
			h.headers(1, true, connect(":scheme", "https", ":authority", "x", ":path", "/ws")...)
		}},
		{"extended connect with data", func(h *h2conn) {
			h.headers(1, false, connect(":scheme", "https", ":authority", "x", ":path", "/ws", "x-a", "1")...)
			h.frame(0, 1, 1, []byte("abc"))
		}},
		{"extended connect with a query", func(h *h2conn) {
			h.headers(1, true, connect(":scheme", "https", ":authority", "x", ":path", "/ws?a=1")...)
		}},
		{"extended connect without :path", func(h *h2conn) {
			h.headers(1, true, connect(":scheme", "https", ":authority", "x")...)
		}},
		{"extended connect without :scheme", func(h *h2conn) {
			h.headers(1, true, connect(":authority", "x", ":path", "/ws")...)
		}},
		{"extended connect without :authority", func(h *h2conn) {
			h.headers(1, true, connect(":scheme", "https", ":path", "/ws")...)
		}},
		{"extended connect with a relative path", func(h *h2conn) {
			h.headers(1, true, connect(":scheme", "https", ":authority", "x", ":path", "ws")...)
		}},
		{"connect with an empty :protocol", func(h *h2conn) {
			h.headers(1, true, ":method", "CONNECT", ":protocol", "", ":scheme", "https", ":authority", "x", ":path", "/ws")
		}},
		{"plain connect", func(h *h2conn) { h.headers(1, true, ":method", "CONNECT", ":authority", "x:443") }},
		{"get with :protocol", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello", ":protocol", "websocket")...) }},
		{"get without :protocol", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello")...) }},
	}
	for _, c := range cases {
		h, err := h2Dial(addr, p)
		if err != nil {
			fmt.Fprintf(&out, "%s: dial: %v\n", c.name, err)
			continue
		}
		c.send(h)
		fmt.Fprintf(&out, "%s: %s\n", c.name, h2Answer(h))
	}
	if native {
		cmd.Process.Signal(os.Interrupt)
	}
	return out.String()
}

// h2ServerSettings reads the server's first SETTINGS frame and names
// RFC 8441's entry: "ENABLE_CONNECT_PROTOCOL=v", or "none".
func h2ServerSettings(addr string, p *testPKI) string {
	h, err := h2DialRaw(addr, p)
	if err != nil {
		return "dial: " + err.Error()
	}
	defer h.c.Close()
	h.c.SetReadDeadline(time.Now().Add(h2EventsMax))
	for {
		typ, flags, _, pl, err := h.read()
		if err != nil {
			return "read: " + err.Error()
		}
		if typ != 4 || flags&1 != 0 {
			continue
		}
		for i := 0; i+6 <= len(pl); i += 6 {
			if binary.BigEndian.Uint16(pl[i:]) == 8 {
				return fmt.Sprintf("ENABLE_CONNECT_PROTOCOL=%d", binary.BigEndian.Uint32(pl[i+2:]))
			}
		}
		return "none"
	}
}

// h2DialRaw is h2Dial without reading the server's SETTINGS: the client
// preface and an empty SETTINGS sent, nothing read.
func h2DialRaw(addr string, p *testPKI) (*h2conn, error) {
	c, err := tls.Dial("tcp", addr, &tls.Config{RootCAs: p.roots, NextProtos: []string{"h2"}})
	if err != nil {
		return nil, err
	}
	h := &h2conn{c: c, br: bufio.NewReader(c)}
	h.write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
	h.frame(4, 0, 0, nil)
	return h, nil
}

// h2Answer reads stream 1's answer: "STATUS BODY" (the DATA joined), or
// "RST_STREAM CODE", "GOAWAY CODE", or the read error; it closes h.
func h2Answer(h *h2conn) string {
	defer h.c.Close()
	h.c.SetReadDeadline(time.Now().Add(h2EventsMax))
	status := ""
	var body []byte
	for {
		typ, flags, sid, pl, err := h.read()
		if err != nil {
			return fmt.Sprintf("%s %s (then %v)", status, body, err)
		}
		switch {
		case typ == 7 && len(pl) >= 8:
			return "GOAWAY " + h2Code(binary.BigEndian.Uint32(pl[4:]))
		case sid != 1:
		case typ == 3 && len(pl) == 4:
			return "RST_STREAM " + h2Code(binary.BigEndian.Uint32(pl))
		case typ == 1:
			status = h2Status(pl)
			if flags&1 != 0 {
				return status
			}
		case typ == 0:
			body = append(body, pl...)
			if flags&1 != 0 {
				return fmt.Sprintf("%s %s", status, body)
			}
		}
	}
}
