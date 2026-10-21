package llvm

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// http1DiffProgram is an HTTP/1.1 server whose handler (a middleware, so
// every request reaches it) describes the request as Xo code sees it, as
// http_diff.c's handler does: the status of /status/<n>, the response
// fields of the query parameters ct, cl, conn, te, a body of big=<n>
// bytes "x" more, or none with empty=1.
const http1DiffProgram = `use std/http

fn describe(req: http.Request) -> http.Response {
  var b = "method=${req.method}\npath=${req.path}\nurl=${req.url.scheme}|${req.url.host}|"
  b = b + (match req.url.port {
    Some(p) => "${p}"
    None => "-"
  })
  b = b + "|${req.url.path}|"
  var qs: List[Str] = []
  for k in req.url.query.keys().sort() {
    qs.push("${k}=${req.url.query.get(k) ?? ""}")
  }
  b = b + qs.join("&") + "\nheaders="
  var hs: List[Str] = []
  for k in req.headers.keys().sort() {
    hs.push("${k}: ${req.headers.get(k) ?? ""}")
    let vs = req.header_values(k)
    // (http_diff_test.go goServe, TestHTTPParseDiff's net/http side,
    // prints the same: change both)
    if vs.len() != 1 || vs[0] != (req.headers.get(k) ?? "") {
      hs.push("${k} fields: ${vs.join(" ;; ")}")
    }
  }
  b = b + hs.join("|") + "\nbody=${req.body.to_hex()}\n"
  var status = 200
  if req.path.starts_with("/status/") && req.path.len() > 8 {
    if let Some(n) = Int.parse(req.path[8..req.path.len()]) {
      status = n
    }
  }
  if let Some(big) = req.url.query.get("big") {
    if let Some(n) = Int.parse(big) {
      if n > 0 {
        b = b + "x".repeat(if n < 1048576 { n } else { 1048576 })
      }
    }
  }
  if req.url.query.get("empty") is Some(_) {
    b = ""
  }

  var resp = http.Response{status: status, headers: {}, body: b.bytes()}
  for (q, name) in [("ct", "Content-Type"), ("cl", "Content-Length"), ("conn", "Connection"), ("te", "Transfer-Encoding")] {
    if let Some(v) = req.url.query.get(q) {
      resp = resp.with_header(name, v)
    }
  }
  resp
}

fn main(os: Os) {
  let l = os.net.listen("127.0.0.1:0").or_fault("listen")
  var r = http.router()
  r.wrap(fn(req, _next) { describe(req) })
  os.stdio.println("ADDR ${l.addr()}")
  match http.server(l, r).serve() {
    Ok(_) => ()
    Err(e) => os.stdio.println("serve: ${e}")
  }
}
`

// http1Server runs a built server program: its address and a stop.
func http1Server(t testing.TB, bin string, env ...string) (string, func()) {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	return startHTTP1Server(t, cmd)
}

// startHTTP1Server starts cmd, a server that prints "ADDR host:port"
// first, behind a lifeline, and records its exit for http1Send (which
// stops dialing an address whose server has exited); the address and a
// stop that kills it and waits.
func startHTTP1Server(t testing.TB, cmd *exec.Cmd) (string, func()) {
	t.Helper()
	testtmp.Lifeline(t, cmd, 30*time.Minute) // no orphan if the test is killed
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	life := &serverLife{done: make(chan struct{})}
	cmd.Stderr = &life.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	rd := bufio.NewReader(stdout)
	line, err := rd.ReadString('\n')
	addr, ok := strings.CutPrefix(strings.TrimSpace(line), "ADDR ")
	if err != nil || !ok {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("%s: %q %v\n%s", cmd.Path, line, err, clip(life.stderr.String()))
	}
	servers.Store(addr, life)
	go func() {
		io.Copy(io.Discard, rd)
		life.err = cmd.Wait()
		close(life.done)
	}()
	return addr, func() {
		cmd.Process.Kill()
		<-life.done
		servers.CompareAndDelete(addr, life)
	}
}

// serverLife is a native server's exit: done closes when its process
// has exited, with Wait's error and what it wrote to standard error.
type serverLife struct {
	done   chan struct{}
	err    error
	stderr lockedBuffer
}

// servers maps the address of each server startHTTP1Server started to
// its serverLife.
var servers sync.Map

// serverExited is the exit of the server at addr when it has exited
// (its status and the end of its standard error), else "".
func serverExited(addr string) string {
	v, ok := servers.Load(addr)
	if !ok {
		return ""
	}
	life := v.(*serverLife)
	select {
	case <-life.done:
		return fmt.Sprintf("server exited (%v)\n%s", life.err, clip(life.stderr.String()))
	default:
		return ""
	}
}

// lockedBuffer is a bytes.Buffer safe for a writer goroutine and readers.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// http1Send sends input on a connection of its own, closes the sending
// side, and returns what the server answered until it closed. A reset
// (the server closed with input unread) is reported as such.
func http1Send(addr string, input string) string {
	var c net.Conn
	var err error
	for try := 0; ; try++ {
		// a private local port: its TIME_WAIT (this side closes first)
		// takes none of the machine's ephemeral ports (xotool ports.go)
		c, err = xotool.DialLoopback(addr, 10*time.Second)
		if err == nil || try == 300 {
			break
		}
		// a server that has exited never answers: fail now, not after a
		// minute of retries per input (FuzzHTTPDiff, 2026-10-16)
		if e := serverExited(addr); e != "" {
			return "dial: " + e
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		return "dial: " + err.Error()
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(30 * time.Second))
	go func() {
		io.WriteString(c, input)
		c.(*net.TCPConn).CloseWrite()
	}()
	out, err := io.ReadAll(c)
	if err != nil {
		if errors.Is(err, syscall.ECONNRESET) {
			return string(out) + "<reset>"
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return string(out) + "<timeout>"
		}
		return string(out) + "<" + err.Error() + ">"
	}
	return string(out)
}

// http1DiffInputs are the inputs of TestHTTP1ParseDiff: the corpus, the
// header limits, and n random requests, every second one mutated.
func http1DiffInputs(n int, seed int64) []string {
	inputs := append([]string(nil), httpCorpus...)
	inputs = append(inputs,
		"GET / HTTP/1.1\r\nHost: x\r\nX-Big: "+strings.Repeat("a", 1<<20)+"\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nX-Big: "+strings.Repeat("a", 1<<20+8192)+"\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\n"+strings.Repeat("X-Many: v\r\n", 5000)+"\r\n",
		"GET /"+strings.Repeat("p", 70000)+" HTTP/1.1\r\nHost: x\r\n\r\n",
		"OPTIONS * HTTP/1.1\r\nHost: x\r\nContent-Length: 5000\r\n\r\n"+strings.Repeat("a", 5000),
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n"+strings.Repeat("1000\r\n"+strings.Repeat("c", 4096)+"\r\n", 40)+"0\r\n\r\n",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n"+strings.Repeat("1;"+strings.Repeat("e", 100)+"\r\nc\r\n", 200)+"0\r\n\r\n",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\nffffffffffffffff\r\nabc",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n10000000000000000\r\nabc",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\nX-T: 1\r\nBad Line\r\n\r\n",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: Chunked\r\n\r\n3 \t; x=1\r\nabc\r\n0\r\n\r\n",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n3\nabc\r\n0\r\n\r\n",
		"GET http://[::1]:8080/a?b=c HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://[fe80::1%25en0]/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://[::ffff:1.2.3.4]/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://[1:2:3:4:5:6:7:8:9]/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://[::1.2.3.04]/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://u:p@h/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://u%zz@h/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://h:x/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://h:1:2/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://h%41/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://h%C3%A9/ HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET HTTP://H.example:99999999999999999999/x HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET //h/p HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET /a?x=1&x=2&y&=z&%zz=1&a+b=c+d;e HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET /a? HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET /a?? HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET mailto:x HTTP/1.1\r\nHost: x\r\n\r\n",
		"CONNECT /p HTTP/1.1\r\nHost: x\r\n\r\n",
		"CONNECT h:1 HTTP/1.1\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: [::1]\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: a:1:2\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: [::1]x:80\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nPragma: no-cache\r\nCache-Control: max-age=0\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nX-A: 1\r\nx-a: 2\r\nX-A:3\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nfoo-bar_baz: 1\r\nCONTENT-type: t\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nConnection: Keep-Alive, Upgrade\r\nExpect: 100-CONTINUE\r\n\r\n",
		"GET / HTTP/1.0\r\nConnection: x, keep-alive\r\n\r\nGET /2 HTTP/1.0\r\nConnection: close\r\n\r\n",
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length:  7 \r\n\r\nabcdefg",
		// (a body cut short with 256 KiB or more unread is left out: the
		// native server closes after it as net/http alone does, the Go
		// backend does not; gotchas/http-short-body-close-differs.md)
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 9223372036854775807\r\nExpect: 100-continue\r\nX: \x01\r\n\r\n",
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 9223372036854775808\r\n\r\nab",
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length: +3\r\n\r\nabc",
		"GET / HTTP/0.0\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
		"GET / HTTP/9.9\r\nHost: x\r\n\r\n",
		"PRI * HTTP/2.0\r\nX: y\r\n\r\n",
		"GET /caf%C3%A9?q=%E2%82%AC HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET /\xff?\xfe=\xfd HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nX-Long: a\r\n b\r\n\t c\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nX-Long: a\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nX-Long: a\r\n  ",
		"\r\n\r\nGET / HTTP/1.1\r\nHost: x\r\n\r\n",
		strings.Repeat("a", 4096),
		"GET / HTTP/1.1\r\nHost: x\r\n\r\nPOST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 2\r\n\r\nab\r\n\r\nGET /3 HTTP/1.1\r\nHost: x\r\n\r\n",
	)
	inputs = append(inputs, http1HeaderInputs(seed)...)
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < n; i++ {
		s := randRequest(r)
		if i%2 == 1 {
			s = string(mutateHTTP(r, []byte(s)))
		}
		inputs = append(inputs, s)
	}
	return inputs
}

// http1HeaderInputs are header block cases (net/textproto's
// ReadMIMEHeader and net/http's readRequest: repeated names in any case,
// continuation lines, names with spaces, colons, and non-token bytes,
// control and non-UTF-8 bytes in values, Host, Content-Length, and
// Transfer-Encoding repeated or malformed) and 400 random header blocks
// of such names and values (2026-10-21; this corpus found the native
// servers joining Set-Cookie fields, std/changes/http.md).
func http1HeaderInputs(seed int64) []string {
	end := "Connection: close\r\n\r\n"
	h := "GET /p?q=1 HTTP/1.1\r\nHost: x\r\n"
	cases := []string{
		h + "X-A: 1\r\nx-a: 2\r\n" + end,
		h + "X-Long:  spaced value  \r\n" + end,
		h + "X-Fold: a\r\n b\r\n" + end,
		h + "X-Fold: a\r\n\tb\r\n" + end,
		h + "X-Fold: a\r\n  \r\n" + end,
		h + "X-Fold:\r\n b\r\n" + end,
		h + " X-Lead: 1\r\n" + end,
		h + "X-No-Colon\r\n" + end,
		h + "X-Space : 1\r\n" + end,
		h + "X Sp: 1\r\n" + end,
		h + ": empty-name\r\n" + end,
		h + "X-Empty:\r\n" + end,
		h + "X-Ctl: a\x01b\r\n" + end,
		h + "X-Del: a\x7fb\r\n" + end,
		h + "X-Tab: a\tb\r\n" + end,
		h + "X-High: caf\xc3\xa9\r\n" + end,
		h + "X-Bad-Utf8: \xff\xfe\r\n" + end,
		h + "X-Nul: a\x00b\r\n" + end,
		h + "X-\xc3\xa9: v\r\n" + end,
		h + "x-lower-case: v\r\n" + end,
		h + "X-UNDER_SCORE: v\r\n" + end,
		h + "x-a-b-c: 1\r\nX-a-B-c: 2\r\n" + end,
		h + "Content-Type: text/plain\r\ncontent-type: text/html\r\n" + end,
		h + "Set-Cookie: a=1\r\nSet-Cookie: b=2\r\n" + end,
		h + "Cookie: a=1\r\nCookie: b=2\r\n" + end,
		h + "Accept: a\r\nAccept: \r\nAccept: b\r\n" + end,
		h + "X-Colon: a:b: c\r\n" + end,
		h + "X-A:1\r\nX-A:\t2\t\r\n" + end,
		"GET /p HTTP/1.1\r\n" + end,
		"GET /p HTTP/1.1\r\nHost: a\r\nHost: b\r\n" + end,
		"GET /p HTTP/1.1\r\nHost: a b\r\n" + end,
		"GET /p HTTP/1.1\r\nHost: a/b\r\n" + end,
		"GET /p HTTP/1.1\r\nhost: lower\r\n" + end,
		"GET /p HTTP/1.1\r\nHost:\r\n" + end,
		"GET /p HTTP/1.0\r\n" + end,
		"GET /p HTTP/1.1\nHost: x\n" + "Connection: close\n\n",
		"GET /p HTTP/1.1\r\nHost: x\nX-Mixed: 1\r\n" + end,
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nContent-Length: 3\r\n" + end + "abc",
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nContent-Length: 4\r\n" + end + "abcd",
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 3, 3\r\n" + end + "abc",
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: -1\r\n" + end,
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 0x3\r\n" + end + "abc",
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length:\r\n" + end,
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: gzip, chunked\r\n" + end + "3\r\nabc\r\n0\r\n\r\n",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: identity\r\n" + end,
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nContent-Length: 3\r\n" + end + "3\r\nabc\r\n0\r\n\r\n",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: chunked\r\n" + end + "3\r\nabc\r\n0\r\n\r\n",
		"POST /p HTTP/1.0\r\nHost: x\r\nTransfer-Encoding: chunked\r\n" + end + "3\r\nabc\r\n0\r\n\r\n",
		"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n" + end + "3\r\nabc\r\n0\r\nTrailer-A: t\r\ntrailer-a: u\r\n\r\n",
		"GET /p HTTP/1.1\r\nHost: x\r\nExpect: something\r\n" + end,
		"GET /p HTTP/1.1\r\nHost: x\r\nConnection: close\r\nConnection: keep-alive\r\n\r\n",
		"GET /p HTTP/1.1\r\nHost: x\r\n" + strings.Repeat("X-Many: v\r\n", 100) + end,
		"GET /p HTTP/1.1\r\nHost: x\r\nX-Fold: " + strings.Repeat("a\r\n ", 2000) + "b\r\n" + end,
	}
	names := []string{"X-A", "x-a", "Content-Type", "Host", "X-\xc3\x9cn\xc3\xaf", "X A", "X:B", "", "X-B", "Accept", "Set-Cookie", "Cookie", "X_C", "X-\t"}
	vals := []string{"1", " 2 ", "a,b", "", "\t", "x\x01", "\xc3\xa9", "  ", "a\r\n b", "v:w"}
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < 400; i++ {
		var b strings.Builder
		b.WriteString(h)
		n := 1 + r.Intn(4)
		for k := 0; k < n; k++ {
			b.WriteString(names[r.Intn(len(names))])
			b.WriteString([]string{":", ": ", " :", ":"}[r.Intn(4)])
			b.WriteString(vals[r.Intn(len(vals))])
			b.WriteString("\r\n")
		}
		b.WriteString(end)
		cases = append(cases, b.String())
	}
	return cases
}

// TestHTTP1ParseDiff is the differential test of the native HTTP/1.1
// server's request parsing in Xo (std/native/llrt/http1.xo, decision 0193
// step 6) against the Go backend (net/http): the same server program
// built with every backend answers the same connections (the corpus of
// TestHTTPParseDiff, malformed requests, header limits, chunked bodies
// and trailers, request targets, mutated random requests), each sent on
// a connection of its own whose sending side then closes; the answers
// must be equal byte for byte (Date values aside). XO_HTTP_DIFF_N sets
// the number of random requests (default 2000).
func TestHTTP1ParseDiff(t *testing.T) {
	xotool.NeedPorts(t)
	if testing.Short() {
		t.Skip("builds an HTTP server with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(http1DiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	addrs := make([]string, len(backends))
	for i, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil {
			t.Fatalf("build (%s): %v", be, err)
		}
		if res.Failed {
			t.Fatalf("build (%s) failed: %v", be, res.Output)
		}
		addr, stop := http1Server(t, res.Binaries[xotool.Host()])
		defer stop()
		addrs[i] = addr
	}
	n := 2000
	if v, err := strconv.Atoi(os.Getenv("XO_HTTP_DIFF_N")); err == nil {
		n = v
	}
	seed := int64(7)
	if v, err := strconv.ParseInt(os.Getenv("XO_HTTP_DIFF_SEED"), 10, 64); err == nil {
		seed = v
	}
	inputs := http1DiffInputs(n, seed)
	outs := make([][]string, len(inputs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i, in := range inputs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := make([]string, len(addrs))
			// an answer cut by a reset is retried: whether the reset
			// overtakes the answer is timing
			for try := 0; try < 4; try++ {
				for b, a := range addrs {
					res[b] = normDate(http1Send(a, in))
				}
				reset := false
				for _, s := range res {
					reset = reset || strings.HasSuffix(s, "<reset>")
				}
				if !reset {
					break
				}
			}
			outs[i] = res
		}()
	}
	wg.Wait()
	bad := 0
	for i, in := range inputs {
		for b := 1; b < len(backends); b++ {
			if outs[i][b] != outs[i][0] {
				bad++
				if bad <= 15 {
					shown := in
					if len(shown) > 300 {
						shown = shown[:300] + fmt.Sprintf("... (%d bytes)", len(in))
					}
					got, want := outs[i][b], outs[i][0]
					if len(got) > 2000 {
						got = got[:2000] + "..."
					}
					if len(want) > 2000 {
						want = want[:2000] + "..."
					}
					t.Errorf("input %q:\n--- %s\n%s\n--- go\n%s", shown, backends[b], got, want)
				}
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d answers of %d inputs differ", bad, len(inputs))
	}
	t.Logf("%d inputs, backends %v", len(inputs), backends)
}

// exitingServer is the test binary as a server that prints the address
// of a port nothing listens on and exits 3 (XO_TEST_EXITING_SERVER=1,
// TestMain): TestHTTP1SendServerExited's.
func exitingServer() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(2)
	}
	addr := l.Addr().String()
	l.Close()
	fmt.Println("ADDR " + addr)
	fmt.Fprintln(os.Stderr, "exiting server: gone")
	os.Exit(3)
}

// TestHTTP1SendServerExited: http1Send to a server that has exited
// fails at once with its exit status and standard error, instead of
// retrying the dial for a minute (FuzzHTTPDiff sat 20 minutes on a dead
// server, one minute per seed input, 2026-10-16).
func TestHTTP1SendServerExited(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "XO_TEST_EXITING_SERVER=1")
	addr, stop := startHTTP1Server(t, cmd)
	defer stop()
	start := time.Now()
	got := http1Send(addr, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("http1Send took %v on an exited server", d)
	}
	if !strings.HasPrefix(got, "dial: server exited") || !strings.Contains(got, "exit status 3") || !strings.Contains(got, "exiting server: gone") {
		t.Fatalf("http1Send: %q", got)
	}
}
