package llvm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestH2Diff runs the same HTTPS program over HTTP/2 (decision 0121)
// built with the Go backend (net/http's HTTP/2) and the native backends
// (std/native/llrt/h2srv.xo) and requires the same observations:
//
//   - as a client (the program's output): a Go HTTP/2 server echoing
//     what it received (the protocol, User-Agent, Accept-Encoding,
//     Content-Length, cookies split into fields), a 3 MB upload and a
//     5 MB download (flow control both ways), streams the server resets
//     before and after the response header, and a Connection header
//     HTTP/2 refuses; and the program's own server: statuses, sniffed
//     types, Content-Length, HEAD, a fault (500), a streamed body and a
//     fault in one (a reset stream), sorted fields, and 20 requests at
//     once;
//   - as a server (probes of this test): net/http's HTTP/2 client (the
//     same requests, 200 streams at once on one connection, more than
//     the 250 stream limit), and raw frames written here (h2spec style):
//     connection and stream errors with their codes, HPACK errors,
//     flow control errors, a header list over the limit (431),
//     connection specific fields (400), trailers, PING, a rapid reset
//     attack (ENHANCE_YOUR_CALM), and a stop request (SIGTERM) with a
//     stream in flight (GOAWAY NO_ERROR, the stream finishes).
//
// The native binaries run with XO_LEAKCHECK=1.
func TestH2Diff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an HTTP/2 program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	pki := newTestPKI(t)
	peer := startH2Peer(t, pki)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(h2DiffProgram(pki, peer)), 0o644); err != nil {
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
				outs[i], _ = runH2Diff(t, res.Binaries[xotool.Host()], pki, be != xotool.BackendGo)
				outs[i] = rejectRace(outs[i])
			})
		}
	})
	if t.Failed() {
		return
	}
	want := outs[0]
	t.Logf("go backend:\n%s", want)
	for i, be := range backends[1:] {
		if got := outs[i+1]; got != want {
			t.Errorf("%s backend differs from the Go backend\n%s", be, lineDiff(want, got))
		}
	}
}

// rejectRace makes the two results of the cbc suite probe one on
// Windows: a server that rejects the connection writes its GOAWAY and
// closes with the client's preface unread, which Windows answers with a
// reset that can overtake the GOAWAY (the Go backend's server loses it
// now and then; the native one lingers, decision 0197). Elsewhere the
// output is as it is.
func rejectRace(out string) string {
	if runtime.GOOS != "windows" {
		return out
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "raw cbc suite: ") && strings.HasSuffix(l, "closed") {
			lines[i] = "raw cbc suite: GOAWAY INADEQUATE_SECURITY or a reset, closed"
		}
	}
	return strings.Join(lines, "\n")
}

// lineDiff shows the lines of got that differ from want.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, c string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			c = g[i]
		}
		if a != c {
			fmt.Fprintf(&b, "line %d\n  go:     %s\n  native: %s\n", i+1, a, c)
		}
	}
	return b.String()
}

// startH2Peer starts the Go HTTP/2 server the program calls: its address.
func startH2Peer(t *testing.T, p *testPKI) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/rstearly", func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) })
	mux.HandleFunc("/rstlate", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("r", 10000)))
		w.(http.Flusher).Flush()
		time.Sleep(50 * time.Millisecond)
		panic(http.ErrAbortHandler)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("g", 5<<20)))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Add("X-Multi", "a")
		w.Header().Add("X-Multi", "b")
		fmt.Fprintf(w, "proto=%s method=%s path=%s query=%s ua=%q ae=%q cl=%q ct=%q cookie=%q custom=%q body=%d", r.Proto, r.Method, r.URL.Path,
			r.URL.RawQuery, r.Header.Get("User-Agent"), r.Header.Get("Accept-Encoding"), r.Header.Get("Content-Length"),
			r.Header.Get("Content-Type"), r.Header["Cookie"], r.Header.Get("X-Custom"), len(body))
	})
	srv := &http.Server{Handler: mux, ErrorLog: log.New(io.Discard, "", 0),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{p.goodTLS}}}
	go srv.ServeTLS(l, "", "")
	t.Cleanup(func() { srv.Close() })
	return l.Addr().String()
}

// h2DiffProgram is the Xo program: client calls of the peer and of its
// own server, then an HTTPS server for the probes until a stop request.
func h2DiffProgram(p *testPKI, peer string) string {
	var b strings.Builder
	b.WriteString(`use std/http

fn describe(e: http.ClientErr, peer: Str, own: Str) -> Str {
  let s = match e {
    http.ClientErr.InvalidUrl(msg) => "InvalidUrl(${msg})"
    http.ClientErr.Connect(msg) => "Connect(${msg})"
    http.ClientErr.Timeout => "Timeout"
    http.ClientErr.Protocol(msg) => "Protocol(${msg})"
    http.ClientErr.Canceled => "Canceled"
  }
  s.replace(peer, "<peer>").replace(own, "<own>")
}

/// The fields of h sorted by name, without Date.
fn fields(h: Map[Str, Str]) -> Str {
  var out: List[Str] = []
  for k in h.keys().sort() {
    if k != "Date" {
      out.push("${k}=${h.get(k) ?? ""}")
    }
  }
  out.join(" ")
}

fn body_of(b: Bytes) -> Str {
  if b.len() <= 300 {
    return b.to_str() ?? "<not utf-8>"
  }
  "<${b.len()} bytes>"
}

fn show(os: Os, label: Str, peer: Str, own: Str, r: Result[http.Response, http.ClientErr]) {
  match r {
    Ok(resp) => os.stdio.println("${label}: ${resp.status} [${fields(resp.headers)}] ${body_of(resp.body)}")
    Err(e) => os.stdio.println("${label}: ${describe(e, peer, own)}")
  }
}

fn routes(clock: Clock) -> http.Router uses clock {
  var r = http.router()
  // RFC 7541's examples request / and /index.html (probeH2HPACK)
  r.handle("GET /", fn(req) { http.text(200, "${fields(req.headers)} host=${req.url.host} scheme=${req.url.scheme}") })
  r.handle("GET /index.html", fn(req) { http.text(200, "${fields(req.headers)} host=${req.url.host} scheme=${req.url.scheme}") })
  r.handle("GET /hello", fn(req) { http.text(200, "hello ${req.method} ${req.path} q=${req.query("q") ?? "-"}") })
  r.handle("/any", fn(req) { http.text(200, "any ${req.method}") })
  r.handle("/headers", fn(req) {
    var h = req.headers
    // the Go client's User-Agent and the native one's agree; the rest too
    http.text(200, "${fields(h)} host=${req.url.host} scheme=${req.url.scheme} port=${req.url.port ?? -1}")
  })
  r.handle("POST /echo", fn(req) {
    let ok = req.body == "z".repeat(req.body.len()).bytes()
    http.text(200, "len=${req.body.len()} ok=${ok}")
  })
  r.handle("GET /big/{n:Int}", fn(req) { http.text(200, "b".repeat(req.param_int("n"))) })
  r.handle("GET /bytes/{n:Int}", fn(req) { http.empty(200).with_header("X-N", "${req.param_int("n")}") })
  r.handle("GET /status/{code:Int}", fn(req) { http.empty(req.param_int("code")) })
  r.handle("GET /json", fn(_req) { http.json(201, [1, 2, 3]) })
  r.handle("GET /html", fn(_req) { http.text(200, "<html><body>hi</body></html>").with_header("Content-Type", "") })
  r.handle("GET /fault", fn(_req) {
    let words = ["a", "b", "c"]
    if words.len() > 2 {
      fault("handler gave up")
    }
    http.text(200, "never")
  })
  r.handle("GET /stream", fn(_req) {
    http.stream(200, fn(w) {
      w.write("one,".bytes())?
      w.flush()?
      w.write("x".repeat(10000).bytes())?
      w.write(",end".bytes())?
    })
  })
  r.handle("GET /smallstream", fn(_req) {
    http.stream(200, fn(w) {
      w.write("small".bytes())?
      w.write(" stream".bytes())?
    })
  })
  r.handle("GET /flushfirst", fn(_req) {
    http.stream(202, fn(w) {
      w.flush()?
      w.write("{\"a\":1}".bytes())?
    })
  })
  r.handle("GET /streamfault", fn(_req) {
    http.stream(200, fn(w) {
      w.write("y".repeat(5000).bytes())?
      w.flush()?
      let words = ["a", "b", "c"]
      if words.len() > 2 {
        fault("stream gave up")
      }
      Ok(())
    })
  })
  r.handle("GET /slow", fn(_req) {
    clock.sleep(200ms)
    http.text(200, "slow")
  })
  r.handle("GET /fields", fn(_req) {
    http.text(200, "f").with_header("X-B", "2").with_header("X-A", "1").with_header("Content-Type", "text/x").with_header("Keep-Alive", "no")
  })
  r.handle("GET /close", fn(_req) { http.text(200, "bye").with_header("Connection", "close") })
  r.handle("GET /badlen", fn(_req) { http.text(200, "five!").with_header("Content-Length", "abc") })
  r
}

fn main(os: Os) {
`)
	fmt.Fprintf(&b, "  let peer = %q\n", peer)
	fmt.Fprintf(&b, "  let ca = %q.bytes()\n", p.caPEM)
	fmt.Fprintf(&b, "  let cert = http.tls_cert(%q.bytes(), %q.bytes()).or_fault(\"cert\")\n", p.goodCert, p.goodKey)
	b.WriteString(`  let c = http.client(os.net, timeout: 20s).with_roots(ca).or_fault("roots")
  let pb = "https://${peer}"
  // the Go peer
  show(os, "peer get", peer, "-", c.get(pb + "/x?y=1"))
  show(os, "peer post", peer, "-", c.send(http.new_request("POST", pb + "/p", {"Cookie": "a=1; b=2;  c=3", "X-Custom": "v", "Content-Type": "text/plain"}, "data".bytes())))
  show(os, "peer empty post", peer, "-", c.send(http.new_request("POST", pb + "/p", {}, b"")))
  show(os, "peer head", peer, "-", c.send(http.new_request("HEAD", pb + "/h")))
  show(os, "peer ua", peer, "-", c.send(http.new_request("GET", pb + "/u", {"User-Agent": "xo-test", "Accept-Encoding": "identity"})))
  show(os, "peer upload", peer, "-", c.send(http.new_request("PUT", pb + "/up", {}, "u".repeat(3000000).bytes())))
  show(os, "peer download", peer, "-", c.get(pb + "/big"))
  show(os, "peer rst early", peer, "-", c.get(pb + "/rstearly"))
  show(os, "peer rst late", peer, "-", c.get(pb + "/rstlate"))
  show(os, "peer connection header", peer, "-", c.send(http.new_request("GET", pb + "/c", {"Connection": "upgrade"})))
  show(os, "peer connection close", peer, "-", c.send(http.new_request("GET", pb + "/c", {"Connection": "close"})))
  // its own server
  let l = os.net.listen("127.0.0.1:0").or_fault("listen")
  let own = l.addr()
  let base = "https://${own}"
  let srv = http.server(l, routes(os.clock)).with_tls([cert])
  scope s {
    s.spawn_background(fn() {
      match srv.serve() {
        Ok(_) => ()
        Err(e) => os.stdio.println("serve failed: ${e}")
      }
    })
    show(os, "hello", peer, own, c.get(base + "/hello?q=1"))
    show(os, "headers", peer, own, c.send(http.new_request("GET", base + "/headers", {"Cookie": "a=1; b=2", "X-Custom": "v"})))
    show(os, "post headers", peer, own, c.send(http.new_request("POST", base + "/headers", {}, "abc".bytes())))
    show(os, "echo", peer, own, c.send(http.new_request("POST", base + "/echo", {}, "z".repeat(3000000).bytes())))
    for n in [0, 10, 2048, 2049, 4096, 4097, 100000, 1000000] {
      show(os, "big ${n}", peer, own, c.get(base + "/big/${n}"))
    }
    for code in [200, 201, 204, 304, 404, 500] {
      show(os, "status ${code}", peer, own, c.get(base + "/status/${code}"))
    }
    show(os, "json", peer, own, c.get(base + "/json"))
    show(os, "html", peer, own, c.get(base + "/html"))
    show(os, "head", peer, own, c.send(http.new_request("HEAD", base + "/any")))
    show(os, "options", peer, own, c.send(http.new_request("OPTIONS", base + "/any")))
    show(os, "missing", peer, own, c.get(base + "/missing"))
    show(os, "method", peer, own, c.send(http.new_request("DELETE", base + "/hello")))
    show(os, "fault", peer, own, c.get(base + "/fault"))
    show(os, "stream", peer, own, c.get(base + "/stream"))
    show(os, "small stream", peer, own, c.get(base + "/smallstream"))
    show(os, "flush first", peer, own, c.get(base + "/flushfirst"))
    show(os, "stream fault", peer, own, c.get(base + "/streamfault"))
    show(os, "fields", peer, own, c.get(base + "/fields"))
    show(os, "bad length", peer, own, c.get(base + "/badlen"))
    show(os, "close", peer, own, c.get(base + "/close"))
    show(os, "after close", peer, own, c.get(base + "/hello"))
    let start = os.clock.now()
    let oks = scope p {
      var ts: List[Task[Bool, Never]] = []
      for _ in 0..20 {
        ts.push(p.spawn(fn() -> Bool {
          match c.get(base + "/slow") {
            Ok(resp) => resp.status == 200 && resp.body == "slow".bytes()
            Err(_) => false
          }
        }))
      }
      var n = 0
      for t in ts {
        if t.await {
          n += 1
        }
      }
      n
    }
    os.stdio.println("parallel: ${oks} of 20 ok, concurrent=${os.clock.since(start) < 3s}")
  }
  // the server the probes call
  let l2 = os.net.listen("127.0.0.1:0").or_fault("listen")
  os.stdio.println("ADDR ${l2.addr()}")
  match http.server(l2, routes(os.clock)).with_tls([cert]).serve() {
    Ok(_) => os.stdio.println("served")
    Err(e) => os.stdio.println("serve: ${e}")
  }
}
`)
	return b.String()
}

// runH2Diff runs the program (with env added), probes its server,
// stops it during a request, and returns what it printed and what the
// probes saw, and its standard error.
func runH2Diff(t *testing.T, bin string, p *testPKI, native bool, env ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(bin)
	testtmp.Lifeline(t, cmd, 30*time.Minute) // no orphan if the test is killed
	stopAttr(cmd)                            // a stop request reaches it on Windows too
	cmd.Env = append(os.Environ(), env...)
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
	done := false
	defer func() {
		if !done {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	var out strings.Builder
	rd := bufio.NewReader(stdout)
	addr := ""
	for addr == "" {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("program ended before serving: %v\n%s\nstderr:\n%s", err, out.String(), stderr.String())
		}
		if a, ok := strings.CutPrefix(strings.TrimSpace(line), "ADDR "); ok {
			addr = a
			break
		}
		out.WriteString(line)
	}
	out.WriteString("--- probes\n")
	probeH2Client(t, &out, addr, p)
	probeH2Raw(&out, addr, p)
	probeH2HPACK(&out, addr, p)
	// a stop request while a stream is in flight: GOAWAY NO_ERROR, then
	// the stream's response, then the end of the connection
	rc, err := h2Dial(addr, p)
	if err != nil {
		t.Fatal(err)
	}
	rc.headers(1, true, h2Req("GET", "/slow")...)
	time.Sleep(50 * time.Millisecond)
	if err := sendStop(cmd.Process); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&out, "stop with a stream in flight: %s\n", rc.events(5*time.Second))
	rest, _ := io.ReadAll(rd)
	out.Write(rest)
	werr := cmd.Wait()
	done = true
	fmt.Fprintf(&out, "exit: %v\n", werr)
	if native {
		m := leakRE.FindStringSubmatch(stderr.String())
		if m == nil || m[1] != "0" {
			t.Errorf("leak check: %q\nstderr:\n%s", m, stderr.String())
		}
	}
	// handler faults are logged on both backends; nothing else is
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		// the Go backend writes a crash bundle per fault, the native ones
		// none (std.md 4.2)
		if line == "" || strings.Contains(line, "handler fault") || strings.HasPrefix(line, "xo leakcheck:") ||
			strings.HasPrefix(line, "crash bundle: ") {
			continue
		}
		fmt.Fprintf(&out, "stderr: %s\n", line)
	}
	return out.String(), stderr.String()
}

// probeH2Client checks the program's server with net/http's HTTP/2
// client.
func probeH2Client(t *testing.T, out *strings.Builder, addr string, p *testPKI) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: p.roots}, ForceAttemptHTTP2: true}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, Timeout: 20 * time.Second}
	do := func(label, method, path string, body io.Reader, hdr map[string]string) {
		req, _ := http.NewRequest(method, "https://"+addr+path, body)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := hc.Do(req)
		if err != nil {
			fmt.Fprintf(out, "%s: %v\n", label, strings.ReplaceAll(err.Error(), addr, "<addr>"))
			return
		}
		data, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		var keys []string
		for k := range resp.Header {
			if k != "Date" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var fs []string
		for _, k := range keys {
			fs = append(fs, k+"="+strings.Join(resp.Header[k], ","))
		}
		text := string(data)
		if len(data) > 200 {
			text = fmt.Sprintf("<%d bytes>", len(data))
		}
		fmt.Fprintf(out, "%s: %s %s [%s] %q", label, resp.Proto, resp.Status, strings.Join(fs, " "), text)
		if rerr != nil {
			fmt.Fprintf(out, " body error: %v", rerr)
		}
		out.WriteString("\n")
	}
	do("go hello", "GET", "/hello?q=go", nil, nil)
	do("go headers", "GET", "/headers", nil, map[string]string{"X-Custom": "v", "Cookie": "a=1; b=2"})
	do("go echo", "POST", "/echo", strings.NewReader(strings.Repeat("z", 3000000)), nil)
	do("go echo unknown length", "POST", "/echo", io.MultiReader(strings.NewReader(strings.Repeat("z", 100000))), nil)
	do("go big", "GET", "/big/1000000", nil, nil)
	do("go head", "HEAD", "/any", nil, nil)
	do("go 204", "GET", "/status/204", nil, nil)
	do("go 304", "GET", "/status/304", nil, nil)
	do("go fault", "GET", "/fault", nil, nil)
	do("go stream", "GET", "/stream", nil, nil)
	do("go small stream", "GET", "/smallstream", nil, nil)
	do("go flush first", "GET", "/flushfirst", nil, nil)
	do("go stream fault", "GET", "/streamfault", nil, nil)
	do("go fields", "GET", "/fields", nil, nil)
	do("go expect", "POST", "/echo", strings.NewReader("zzz"), map[string]string{"Expect": "100-continue"})
	// streams at once on one connection: 200, then 400 (over the limit of
	// 250 the client waits for streams to end)
	for _, n := range []int{200, 400} {
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok := 0
		protos := map[string]bool{}
		start := time.Now()
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, err := hc.Get("https://" + addr + "/slow")
				if err != nil {
					return
				}
				data, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				mu.Lock()
				protos[resp.Proto] = true
				if string(data) == "slow" {
					ok++
				}
				mu.Unlock()
			}()
		}
		wg.Wait()
		fmt.Fprintf(out, "go %d streams: ok=%d protos=%v concurrent=%v\n", n, ok, protos, time.Since(start) < 5*time.Second)
	}
}

// ---------------------------------------------------------------------------
// Raw HTTP/2 frames (h2spec style)

type h2conn struct {
	c   *tls.Conn
	br  *bufio.Reader
	err error
}

func h2Dial(addr string, p *testPKI) (*h2conn, error) {
	c, err := tls.Dial("tcp", addr, &tls.Config{RootCAs: p.roots, NextProtos: []string{"h2"}})
	if err != nil {
		return nil, err
	}
	if c.ConnectionState().NegotiatedProtocol != "h2" {
		c.Close()
		return nil, fmt.Errorf("ALPN %q", c.ConnectionState().NegotiatedProtocol)
	}
	h := &h2conn{c: c, br: bufio.NewReader(c)}
	h.write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
	h.frame(4, 0, 0, nil) // SETTINGS
	// the server's SETTINGS, acknowledged: its streams limit applies
	c.SetReadDeadline(time.Now().Add(h2EventsMax))
	for {
		typ, flags, _, _, err := h.read()
		if err != nil {
			c.Close()
			return nil, err
		}
		if typ == 4 && flags&1 == 0 {
			break
		}
	}
	h.frame(4, 1, 0, nil)
	return h, nil
}

func (h *h2conn) write(b []byte) {
	if h.err == nil {
		_, h.err = h.c.Write(b)
	}
}

func (h *h2conn) frame(typ, flags byte, sid uint32, payload []byte) {
	hd := make([]byte, 9, 9+len(payload))
	hd[0], hd[1], hd[2] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
	hd[3], hd[4] = typ, flags
	binary.BigEndian.PutUint32(hd[5:], sid)
	h.write(append(hd, payload...))
}

func (h *h2conn) read() (typ, flags byte, sid uint32, payload []byte, err error) {
	var hd [9]byte
	if _, err = io.ReadFull(h.br, hd[:]); err != nil {
		return
	}
	n := int(hd[0])<<16 | int(hd[1])<<8 | int(hd[2])
	payload = make([]byte, n)
	if _, err = io.ReadFull(h.br, payload); err != nil {
		return
	}
	return hd[3], hd[4], binary.BigEndian.Uint32(hd[5:]) & 0x7fffffff, payload, nil
}

// hpackLit encodes fields as literals without indexing (no Huffman).
func hpackLit(fields ...string) []byte {
	var b []byte
	str := func(s string) {
		b = hpackInt(b, 0, 7, len(s))
		b = append(b, s...)
	}
	for i := 0; i+1 < len(fields); i += 2 {
		b = append(b, 0)
		str(fields[i])
		str(fields[i+1])
	}
	return b
}

func hpackInt(b []byte, first byte, prefix, v int) []byte {
	max := 1<<prefix - 1
	if v < max {
		return append(b, first|byte(v))
	}
	b = append(b, first|byte(max))
	v -= max
	for v >= 128 {
		b = append(b, byte(v%128+128))
		v /= 128
	}
	return append(b, byte(v))
}

func h2Req(method, path string, extra ...string) []string {
	return append([]string{":method", method, ":scheme", "https", ":authority", "x", ":path", path}, extra...)
}

func (h *h2conn) headers(sid uint32, end bool, fields ...string) {
	flags := byte(4)
	if end {
		flags |= 1
	}
	h.frame(1, flags, sid, hpackLit(fields...))
}

var h2Codes = []string{"NO_ERROR", "PROTOCOL_ERROR", "INTERNAL_ERROR", "FLOW_CONTROL_ERROR", "SETTINGS_TIMEOUT", "STREAM_CLOSED",
	"FRAME_SIZE_ERROR", "REFUSED_STREAM", "CANCEL", "COMPRESSION_ERROR", "CONNECT_ERROR", "ENHANCE_YOUR_CALM", "INADEQUATE_SECURITY"}

func h2Code(c uint32) string {
	if int(c) < len(h2Codes) {
		return h2Codes[c]
	}
	return fmt.Sprintf("0x%x", c)
}

// h2Status reads :status from the start of a header block (the server
// writes it first): an indexed static entry or a literal.
func h2Status(b []byte) string {
	if len(b) == 0 {
		return "?"
	}
	if b[0]&0x80 != 0 {
		static := map[byte]string{8: "200", 9: "204", 10: "206", 11: "304", 12: "400", 13: "404", 14: "500"}
		if s, ok := static[b[0]&0x7f]; ok {
			return s
		}
		return fmt.Sprintf("index %d", b[0]&0x7f)
	}
	// a literal whose name is a :status entry of the static table (8 to
	// 14): with indexing (0x40), without (0x00), never indexed (0x10)
	idx := b[0] & 0x3f
	if b[0]&0xc0 == 0 {
		idx = b[0] & 0x0f
	}
	if b[0]&0xc0 != 0x80 && idx >= 8 && idx <= 14 && len(b) > 1 {
		n := int(b[1] & 0x7f)
		if len(b) < 2+n {
			return "?"
		}
		v := b[2 : 2+n]
		if b[1]&0x80 == 0 {
			return string(v)
		}
		// Huffman: digits only (5 bits for 0 to 2, 6 for 3 to 9)
		var out []byte
		acc, bits := uint32(0), 0
		for _, x := range v {
			acc = acc<<8 | uint32(x)
			bits += 8
			for bits >= 5 {
				if c := acc >> (bits - 5) & 0x1f; c <= 2 {
					out = append(out, '0'+byte(c))
					bits -= 5
					continue
				}
				if bits < 6 {
					break
				}
				c := acc >> (bits - 6) & 0x3f
				if c < 0x19 || c > 0x1f {
					bits = 0 // padding
					break
				}
				out = append(out, '3'+byte(c-0x19))
				bits -= 6
			}
		}
		return string(out)
	}
	return fmt.Sprintf("literal 0x%02x", b[0])
}

// h2EventsMax bounds how long events waits for the frames a case expects:
// a server that hangs still fails, a slow one (a loaded machine) does not.
const h2EventsMax = 30 * time.Second

// events reads frames and reports what the server sent, per stream in
// the order of their ids (DATA summed), then on the connection (GOAWAY,
// PING acknowledgments) and how it ended. SETTINGS, WINDOW_UPDATE, and
// SETTINGS acknowledgments are left out. Frames of different streams may
// interleave in any order, so only the order within a stream counts.
//
// It reads until the connection ends, or until d has passed and the
// reply is complete: something came, every stream it names has ended
// (END or RST_STREAM), no GOAWAY came (the connection then closes), and
// the report holds every string of until (the streams a case needs).
// When the reply completes after d, it reads d more for the frames that
// follow it. So the result never depends on how fast the server is
// (TestH2Diff printed "open" for a RST_STREAM that came late under
// load). Past h2EventsMax it stops ("open").
func (h *h2conn) events(d time.Duration, until ...string) string {
	defer h.c.Close()
	h.c.SetReadDeadline(time.Now().Add(h2EventsMax))
	type frame struct {
		typ, flags byte
		sid        uint32
		p          []byte
		err        error
	}
	frames := make(chan frame, 64)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			var f frame
			f.typ, f.flags, f.sid, f.p, f.err = h.read()
			select {
			case frames <- f:
			case <-stop:
				return
			}
			if f.err != nil {
				return
			}
		}
	}()
	streams := map[uint32][]string{}
	data := map[uint32]int{}
	var conn []string
	add := func(sid uint32, e string) {
		if n, ok := data[sid]; ok {
			streams[sid] = append(streams[sid], fmt.Sprintf("DATA %d", n))
			delete(data, sid)
		}
		if e != "" {
			streams[sid] = append(streams[sid], e)
		}
	}
	report := func(end string) string {
		ids := map[uint32]bool{}
		for sid := range streams {
			ids[sid] = true
		}
		for sid := range data {
			ids[sid] = true
		}
		var sorted []int
		for sid := range ids {
			sorted = append(sorted, int(sid))
		}
		sort.Ints(sorted)
		var out []string
		for _, sid := range sorted {
			evs := streams[uint32(sid)]
			if n, ok := data[uint32(sid)]; ok {
				evs = append(evs[:len(evs):len(evs)], fmt.Sprintf("DATA %d", n))
			}
			out = append(out, fmt.Sprintf("%d[%s]", sid, strings.Join(evs, " ")))
		}
		out = append(out, conn...)
		out = append(out, end)
		return strings.Join(out, ", ")
	}
	met := func() bool {
		if len(streams)+len(data)+len(conn) == 0 || len(data) > 0 {
			return false // nothing yet, or a body without its end
		}
		for _, evs := range streams {
			last := evs[len(evs)-1]
			if !strings.HasSuffix(last, "END") && !strings.HasPrefix(last, "RST_STREAM") {
				return false
			}
		}
		for _, e := range conn {
			if strings.HasPrefix(e, "GOAWAY") {
				return false
			}
		}
		r := report("")
		for _, u := range until {
			if !strings.Contains(r, u) {
				return false
			}
		}
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	waiting := false // d has passed, until is not met yet
	end := ""
	for end == "" {
		select {
		case <-timer.C:
			if met() {
				end = "open"
			} else {
				waiting = true
			}
			continue
		case f := <-frames:
			if f.err != nil {
				var ne net.Error
				if errors.As(f.err, &ne) && ne.Timeout() {
					end = "open"
				} else {
					end = "closed"
				}
				continue
			}
			switch f.typ {
			case 0:
				data[f.sid] += len(f.p)
				if f.flags&1 != 0 {
					add(f.sid, "END")
				}
			case 1:
				blk := f.p
				if f.flags&8 != 0 {
					blk = blk[1 : len(blk)-int(f.p[0])]
				}
				if f.flags&0x20 != 0 {
					blk = blk[5:]
				}
				e := "HEADERS " + h2Status(blk)
				if f.flags&1 != 0 {
					e += " END"
				}
				add(f.sid, e)
			case 3:
				add(f.sid, "RST_STREAM "+h2Code(binary.BigEndian.Uint32(f.p)))
			case 6:
				if f.flags&1 != 0 {
					conn = append(conn, fmt.Sprintf("PING ACK %q", f.p))
				}
			case 7:
				e := fmt.Sprintf("GOAWAY last=%d %s", binary.BigEndian.Uint32(f.p)&0x7fffffff, h2Code(binary.BigEndian.Uint32(f.p[4:])))
				if len(f.p) > 8 {
					e += fmt.Sprintf(" %q", f.p[8:])
				}
				conn = append(conn, e)
			}
			if waiting && met() {
				waiting = false
				timer.Reset(d)
			}
		}
	}
	return report(end)
}

// probeH2Raw sends raw frames to the program's server and reports what
// came back for each case.
func probeH2Raw(out *strings.Builder, addr string, p *testPKI) {
	type rawCase struct {
		name string
		send func(h *h2conn)
		wait time.Duration
	}
	settingsFrame := func(id uint16, v uint32) []byte {
		b := make([]byte, 6)
		binary.BigEndian.PutUint16(b, id)
		binary.BigEndian.PutUint32(b[2:], v)
		return b
	}
	u32 := func(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
	cases := []rawCase{
		{"get", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello")...) }, 0},
		{"ping", func(h *h2conn) { h.frame(6, 0, 0, []byte("12345678")) }, 300 * time.Millisecond},
		{"two streams", func(h *h2conn) {
			h.headers(1, true, h2Req("GET", "/slow")...)
			h.headers(3, true, h2Req("GET", "/hello")...)
		}, 0},
		{"post with body and trailers", func(h *h2conn) {
			h.headers(1, false, h2Req("POST", "/echo")...)
			h.frame(0, 0, 1, []byte("zzzz"))
			h.headers(1, true, "x-trailer", "1")
		}, 0},
		{"trailers with a pseudo field", func(h *h2conn) {
			h.headers(1, false, h2Req("POST", "/echo")...)
			h.headers(1, true, ":path", "/x")
		}, 0},
		{"content-length too short", func(h *h2conn) {
			h.headers(1, false, h2Req("POST", "/echo", "content-length", "2")...)
			h.frame(0, 1, 1, []byte("zzzz"))
		}, 0},
		{"content-length too long", func(h *h2conn) {
			h.headers(1, false, h2Req("POST", "/echo", "content-length", "10")...)
			h.frame(0, 1, 1, []byte("zzzz"))
		}, 0},
		{"expect 100-continue", func(h *h2conn) {
			h.headers(1, false, h2Req("POST", "/echo", "expect", "100-continue")...)
			time.Sleep(100 * time.Millisecond)
			h.frame(0, 1, 1, []byte("zz"))
		}, 0},
		{"connection header", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello", "connection", "keep-alive")...) }, 0},
		{"te gzip", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello", "te", "gzip")...) }, 0},
		{"header list too large", func(h *h2conn) {
			// 20000 small fields: 480 KB of block, over 1 MiB of list
			// (32 bytes count per field)
			f := h2Req("GET", "/hello")
			for i := 0; i < 20000; i++ {
				f = append(f, "a", strings.Repeat("v", 20))
			}
			h.frame(1, 5, 1, hpackLit(f...))
		}, 0},
		{"header list far too large", func(h *h2conn) {
			// a CONTINUATION far over what the list may still hold
			var f []string
			for i := 0; i < 40; i++ {
				f = append(f, fmt.Sprintf("x-big-%d", i), strings.Repeat("v", 30000))
			}
			blk := hpackLit(append(h2Req("GET", "/hello"), f...)...)
			h.frame(1, 1, 1, blk[:1<<20])
			h.frame(9, 0, 1, blk[1<<20:1<<20+100000])
		}, 0},
		{"options *", func(h *h2conn) { h.headers(1, true, h2Req("OPTIONS", "*")...) }, 0},
		{"head", func(h *h2conn) { h.headers(1, true, h2Req("HEAD", "/big/100")...) }, 0},
		{"missing :path", func(h *h2conn) { h.headers(1, true, ":method", "GET", ":scheme", "https") }, 0},
		{"bad scheme", func(h *h2conn) { h.headers(1, true, ":method", "GET", ":scheme", "ftp", ":path", "/") }, 0},
		{"unknown pseudo field", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello", ":foo", "1")...) }, 0},
		{"pseudo after regular", func(h *h2conn) { h.headers(1, true, "a", "b", ":method", "GET") }, 0},
		{"upper case name", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello", "X-Up", "1")...) }, 0},
		{"control byte in value", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello", "x-a", "a\x01b")...) }, 0},
		{"userinfo in authority", func(h *h2conn) {
			h.headers(1, true, ":method", "GET", ":scheme", "https", ":authority", "u@x", ":path", "/")
		}, 0},
		{":protocol", func(h *h2conn) { h.headers(1, true, h2Req("GET", "/hello", ":protocol", "websocket")...) }, 0},
		{"even stream", func(h *h2conn) { h.headers(2, true, h2Req("GET", "/hello")...) }, 0},
		{"stream id going down", func(h *h2conn) {
			h.headers(5, true, h2Req("GET", "/slow")...)
			h.headers(3, true, h2Req("GET", "/hello")...)
		}, 0},
		{"data on stream 0", func(h *h2conn) { h.frame(0, 0, 0, []byte("x")) }, 0},
		{"data on an idle stream", func(h *h2conn) { h.frame(0, 0, 7, []byte("x")) }, 0},
		{"data after the end", func(h *h2conn) {
			h.headers(1, true, h2Req("GET", "/slow")...)
			h.frame(0, 0, 1, []byte("x"))
		}, 0},
		{"headers on stream 0", func(h *h2conn) { h.headers(0, true, h2Req("GET", "/hello")...) }, 0},
		{"bad padding", func(h *h2conn) { h.frame(1, 0x8|0x4|0x1, 1, []byte{200, 0x82}) }, 0},
		{"continuation without headers", func(h *h2conn) { h.frame(9, 4, 1, hpackLit(h2Req("GET", "/")...)) }, 0},
		{"other frame inside a header block", func(h *h2conn) {
			h.frame(1, 1, 1, hpackLit(":method", "GET"))
			h.frame(6, 0, 0, []byte("12345678"))
		}, 0},
		{"push promise", func(h *h2conn) { h.frame(5, 4, 1, append(u32(2), hpackLit(h2Req("GET", "/")...)...)) }, 0},
		{"frame too large", func(h *h2conn) {
			// the header of a frame over MAX_FRAME_SIZE (its payload never comes)
			h.write([]byte{0x10, 0, 1, 0, 0, 0, 0, 0, 1})
		}, 0},
		{"ping of 7 bytes", func(h *h2conn) { h.frame(6, 0, 0, []byte("1234567")) }, 0},
		{"ping on a stream", func(h *h2conn) { h.frame(6, 0, 1, []byte("12345678")) }, 0},
		{"settings on a stream", func(h *h2conn) { h.frame(4, 0, 1, nil) }, 0},
		{"settings of 5 bytes", func(h *h2conn) { h.frame(4, 0, 0, []byte{0, 1, 0, 0, 0}) }, 0},
		{"settings ack with data", func(h *h2conn) { h.frame(4, 1, 0, settingsFrame(1, 1)) }, 0},
		{"unrequested settings ack", func(h *h2conn) { h.frame(4, 1, 0, nil); h.frame(4, 1, 0, nil) }, 0},
		{"enable push 2", func(h *h2conn) { h.frame(4, 0, 0, settingsFrame(2, 2)) }, 0},
		{"initial window too large", func(h *h2conn) { h.frame(4, 0, 0, settingsFrame(4, 1<<31)) }, 0},
		{"max frame size too small", func(h *h2conn) { h.frame(4, 0, 0, settingsFrame(5, 100)) }, 0},
		{"duplicate settings", func(h *h2conn) { h.frame(4, 0, 0, append(settingsFrame(3, 1), settingsFrame(3, 2)...)) }, 0},
		{"unknown setting and frame", func(h *h2conn) {
			h.frame(4, 0, 0, settingsFrame(0x99, 1))
			h.frame(0x42, 0, 0, []byte("?"))
			h.headers(1, true, h2Req("GET", "/hello")...)
		}, 0},
		{"window update of 0", func(h *h2conn) { h.frame(8, 0, 0, u32(0)) }, 0},
		{"window update of 0 on a stream", func(h *h2conn) {
			h.headers(1, false, h2Req("POST", "/echo")...)
			h.frame(8, 0, 1, u32(0))
		}, 0},
		{"window update overflow", func(h *h2conn) { h.frame(8, 0, 0, u32(1<<31-1)) }, 0},
		{"stream window overflow", func(h *h2conn) {
			h.headers(1, false, h2Req("POST", "/echo")...)
			h.frame(8, 0, 1, u32(1<<31-1))
		}, 0},
		{"window update on an idle stream", func(h *h2conn) { h.frame(8, 0, 9, u32(1)) }, 0},
		{"window update of 3 bytes", func(h *h2conn) { h.frame(8, 0, 0, []byte{0, 0, 1}) }, 0},
		{"data over the window", func(h *h2conn) {
			// both servers give received bytes back (as the handler reads
			// them, or at once) but hold back less than 4 KiB (inflow), so
			// after 4095 bytes both windows (1 MiB) have 1 MiB - 4095 left
			// whatever the timing; the next frame is 1 byte over them.
			// (65 frames of 16 KiB raced the server's handler: the Go
			// backend reset the stream only when its handler fell behind.)
			h.headers(1, false, h2Req("POST", "/echo")...)
			h.frame(0, 0, 1, make([]byte, 4095))
			h.frame(0, 0, 1, make([]byte, 1<<20-4095+1))
		}, 0},
		{"small window", func(h *h2conn) {
			// the server sends 64 KiB, then waits for WINDOW_UPDATE
			h.frame(4, 0, 0, settingsFrame(4, 1000))
			h.headers(1, true, h2Req("GET", "/big/5000")...)
			time.Sleep(200 * time.Millisecond)
			h.frame(8, 0, 1, u32(10000))
		}, 0},
		{"rst on an idle stream", func(h *h2conn) { h.frame(3, 0, 1, u32(8)) }, 0},
		{"rst of 3 bytes", func(h *h2conn) { h.frame(3, 0, 1, []byte{0, 0, 8}) }, 0},
		{"rst of a request", func(h *h2conn) {
			h.headers(1, true, h2Req("GET", "/slow")...)
			h.frame(3, 0, 1, u32(8))
			h.headers(3, true, h2Req("GET", "/hello")...)
		}, 0},
		{"priority of 4 bytes", func(h *h2conn) { h.frame(2, 0, 1, u32(0)) }, 0},
		{"priority on itself", func(h *h2conn) { h.frame(2, 0, 1, append(u32(1), 16)) }, 0},
		{"headers depending on itself", func(h *h2conn) {
			h.frame(1, 0x20|4|1, 1, append(append(u32(1), 16), hpackLit(h2Req("GET", "/hello")...)...))
		}, 0},
		{"goaway on a stream", func(h *h2conn) { h.frame(7, 0, 1, append(u32(0), u32(0)...)) }, 0},
		{"goaway from the client", func(h *h2conn) {
			h.headers(1, true, h2Req("GET", "/slow")...)
			h.frame(7, 0, 0, append(u32(0), u32(0)...))
		}, 0},
		{"hpack index 0", func(h *h2conn) { h.frame(1, 5, 1, []byte{0x80}) }, 0},
		{"hpack index past the table", func(h *h2conn) { h.frame(1, 5, 1, []byte{0xff, 0x7f}) }, 0},
		{"hpack table size too large", func(h *h2conn) { h.frame(1, 5, 1, hpackInt(nil, 0x20, 5, 5000)) }, 0},
		{"hpack table size after a field", func(h *h2conn) {
			b := append(hpackLit(h2Req("GET", "/hello")...), hpackInt(nil, 0x20, 5, 100)...)
			h.frame(1, 5, 1, b)
		}, 0},
		{"hpack bad huffman", func(h *h2conn) { h.frame(1, 5, 1, []byte{0x82, 0x84, 0x87, 0x41, 0x81, 0x00}) }, 0},
		{"hpack truncated", func(h *h2conn) { h.frame(1, 5, 1, []byte{0x82, 0x00, 0x05, 'a'}) }, 0},
		{"hpack dynamic table", func(h *h2conn) {
			// :authority indexed into the table, then used by index
			h.frame(1, 5, 1, []byte{0x82, 0x84, 0x87, 0x41, 0x01, 'x'})
			h.frame(1, 5, 3, []byte{0x82, 0x84, 0x87, 0xbe})
		}, 0},
		{"settings first", func(h *h2conn) {}, 0},
		{"too many streams", func(h *h2conn) {
			for i := 0; i < 251; i++ {
				h.headers(uint32(2*i+1), false, h2Req("POST", "/echo")...)
			}
		}, 0},
		{"rapid reset", func(h *h2conn) {
			for i := 0; i < 1300; i++ {
				h.headers(uint32(2*i+1), true, h2Req("GET", "/slow")...)
				h.frame(3, 0, uint32(2*i+1), u32(8))
			}
		}, 0},
	}
	// the cases run at once, each on a connection of its own; a case
	// reads for 1.5 s and until its reply is complete (events), so a
	// late frame under load still counts. needs names the streams a
	// case waits for when they may answer in any order.
	needs := map[string][]string{
		"two streams":         {"1[", "3["},
		"hpack dynamic table": {"1[", "3["},
	}
	res := make([]string, len(cases))
	var wg sync.WaitGroup
	for i, rc := range cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var h *h2conn
			var err error
			if rc.name == "settings first" {
				// no SETTINGS: a PING as the first frame
				c, derr := tls.Dial("tcp", addr, &tls.Config{RootCAs: p.roots, NextProtos: []string{"h2"}})
				if derr != nil {
					res[i] = derr.Error()
					return
				}
				h = &h2conn{c: c, br: bufio.NewReader(c)}
				h.write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
				h.frame(6, 0, 0, []byte("12345678"))
			} else if h, err = h2Dial(addr, p); err != nil {
				res[i] = err.Error()
				return
			}
			rc.send(h)
			wait := rc.wait
			if wait == 0 {
				wait = 1500 * time.Millisecond
			}
			res[i] = h.events(wait, needs[rc.name]...)
		}()
	}
	wg.Wait()
	for i, rc := range cases {
		fmt.Fprintf(out, "raw %s: %s\n", rc.name, res[i])
	}
	// a client preface that is wrong: closed without a GOAWAY
	c, err := tls.Dial("tcp", addr, &tls.Config{RootCAs: p.roots, NextProtos: []string{"h2"}})
	if err == nil {
		h := &h2conn{c: c, br: bufio.NewReader(c)}
		h.write([]byte("PRI * HTTP/2.0\r\n\r\nXX\r\n\r\n"))
		fmt.Fprintf(out, "raw bad preface: %s\n", h.events(3*time.Second))
	}
	// TLS 1.2 with a CBC suite: INADEQUATE_SECURITY
	c, err = tls.Dial("tcp", addr, &tls.Config{RootCAs: p.roots, NextProtos: []string{"h2"}, MaxVersion: tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA}})
	if err == nil {
		h := &h2conn{c: c, br: bufio.NewReader(c)}
		h.write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
		h.frame(4, 0, 0, nil)
		fmt.Fprintf(out, "raw cbc suite: %s\n", h.events(3*time.Second))
	} else {
		fmt.Fprintf(out, "raw cbc suite: %v\n", err)
	}
}

// TestH2Sanitizers runs the program of TestH2Diff built by the LLVM
// backend with ThreadSanitizer, and with AddressSanitizer and
// UndefinedBehaviorSanitizer: the same calls and probes (400 streams at
// once on a connection, the raw frame cases, a stop with a stream in
// flight) with no sanitizer report and no allocation live at exit.
func TestH2Sanitizers(t *testing.T) {
	// the TLS library built with sanitizer flags came from the Go build
	// package (tls.go); xo builds it without them, and this test links
	// it itself: port pending (decision 0215, "Tests not carried over")
	t.Skip("decision 0215: needs a sanitizer build of the TLS library")
	if testing.Short() {
		t.Skip("builds an HTTP/2 program with sanitizers")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	pki := newTestPKI(t)
	peer := startH2Peer(t, pki)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(h2DiffProgram(pki, peer)), 0o644); err != nil {
		t.Fatal(err)
	}
	ir, err := xotool.LLVMIR(dir)
	if err != nil {
		t.Fatal(err)
	}
	ll := filepath.Join(t.TempDir(), "prog.ll")
	if err := os.WriteFile(ll, ir, 0o644); err != nil {
		t.Fatal(err)
	}
	srcs := []string{ll}
	flags := []string{"-Wno-override-module"}
	for _, san := range []string{"thread", "address"} {
		t.Run(san, func(t *testing.T) {
			t.Parallel()
			var bin string
			var env []string
			if san == "thread" {
				bin = tsanBuild(t, flags, srcs)
				env = []string{tsanOptions()}
			} else {
				bin = exe(filepath.Join(t.TempDir(), "prog"))
				aflags := append(asanFlags("-O1", "-g", "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined"), flags...)
				if err := xotool.CompileC(aflags, srcs, bin); err != nil {
					if strings.Contains(err.Error(), "unsupported option") || strings.Contains(err.Error(), "sanitizer") {
						t.Skipf("no AddressSanitizer: %v", err)
					}
					t.Fatal(err)
				}
				env = []string{"ASAN_OPTIONS=detect_leaks=0:halt_on_error=1"}
			}
			out, stderr := runH2Diff(t, bin, pki, true, env...)
			for _, bad := range []string{"ThreadSanitizer", "AddressSanitizer", "runtime error:"} {
				if strings.Contains(stderr, bad) {
					if len(stderr) > 8000 {
						stderr = stderr[:8000]
					}
					t.Fatalf("%s report:\n%s", bad, stderr)
				}
			}
			for _, want := range []string{"parallel: 20 of 20 ok", "go 400 streams: ok=400", "\nserved\n", "exit: <nil>"} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}
}
