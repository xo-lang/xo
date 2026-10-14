package llvm

import (
	"bufio"
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
func http1Server(t *testing.T, bin string, env ...string) (string, func()) {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("%s: %v", bin, err)
	}
	addr, ok := strings.CutPrefix(strings.TrimSpace(line), "ADDR ")
	if !ok {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("%s: %q", bin, line)
	}
	return addr, func() {
		cmd.Process.Kill()
		cmd.Wait()
	}
}

// http1Send sends input on a connection of its own, closes the sending
// side, and returns what the server answered until it closed. A reset
// (the server closed with input unread) is reported as such.
func http1Send(addr string, input string) string {
	var c net.Conn
	var err error
	for try := 0; ; try++ {
		c, err = net.DialTimeout("tcp", addr, 10*time.Second)
		if err == nil || try == 300 {
			break
		}
		time.Sleep(200 * time.Millisecond) // ports in TIME_WAIT
	}
	if err != nil {
		return "dial: " + err.Error()
	}
	defer func() {
		// the client closed its side first: a reset at the end leaves no
		// TIME_WAIT behind (thousands of connections run here)
		c.(*net.TCPConn).SetLinger(0)
		c.Close()
	}()
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

// TestHTTP1ParseDiff is the differential test of the native HTTP/1.1
// server's request parsing in Xo (std/native/llrt/http1.xo, decision 0193
// step 6) against the Go backend (net/http): the same server program
// built with every backend answers the same connections (the corpus of
// TestHTTPParseDiff, malformed requests, header limits, chunked bodies
// and trailers, request targets, mutated random requests), each sent on
// a connection of its own whose sending side then closes; the answers
// must be equal byte for byte (Date values aside). XO_LLRT_C=http1 checks
// the C path the same way; XO_HTTP_DIFF_N sets the number of random
// requests (default 2000).
func TestHTTP1ParseDiff(t *testing.T) {
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
	if strings.Contains(","+os.Getenv("XO_LLRT_C")+",", ",http1,") || os.Getenv("XO_LLRT_C") == "all" {
		// the C path's inet_pton (the platform's) takes an IPv4 part
		// with a leading zero, net/url does not
		var kept []string
		for _, in := range inputs {
			if !strings.Contains(in, "[::1.2.3.04]") {
				kept = append(kept, in)
			}
		}
		inputs = kept
	}
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
