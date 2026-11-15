package llvm

import (
	"bufio"
	"bytes"
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
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// http1ClientProgram sends a request for /r/<i> (HEAD when i is a
// multiple of 4) to the server at $ADDR for i from $START (0 when unset)
// below $N with std/http's client and prints what it got: the status,
// the header sorted, the body in hex, or the error's kind and text.
const http1ClientProgram = `use std/http

fn main(os: Os) {
  let addr = os.env.get("ADDR") ?? ""
  let n = Int.parse(os.env.get("N") ?? "0") ?? 0
  let start = Int.parse(os.env.get("START") ?? "0") ?? 0
  let c = http.client(os.net, timeout: 20s)
  for i in start..n {
    let m = if i % 4 == 0 { "HEAD" } else { "GET" }
    match c.send(http.new_request(m, "http://${addr}/r/${i}")) {
      Ok(r) => {
        var hs: List[Str] = []
        for k in r.headers.keys().sort() {
          // every field of the name, as Go's Header.Values: the Map
          // holds a repeated Set-Cookie's first field (decision 0288)
          hs.push("${k}: ${r.header_values(k).join(", ")}")
        }
        os.stdio.println("${i} ${r.status} [${hs.join("|")}] ${r.body.len()} ${r.body.to_hex()}")
      }
      Err(e) => {
        let s = match e {
          http.ClientErr.InvalidUrl(msg) => "InvalidUrl ${msg}"
          http.ClientErr.Connect(msg) => "Connect ${msg}"
          http.ClientErr.Timeout => "Timeout"
          http.ClientErr.Protocol(msg) => "Protocol ${msg}"
          http.ClientErr.Canceled => "Canceled"
          http.ClientErr.Tls(err) => "Protocol ${err.msg}"
        }
        os.stdio.println("${i} ${s}")
      }
    }
  }
}
`

// http1ClientAgree are the responses the native client and the Go
// backend (net/http) read alike before the native client ported
// ReadResponse (2026-10-21); since then every response of the corpus
// is read alike, and these come first in the inputs as before.
var http1ClientAgree = []string{
	"HTTP/1.1 200 OK\r\nContent-Length: 5\r\nX-A: 1\r\n\r\nhello",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n6;ext=1\r\n world\r\n0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nX-B: 2\r\n\r\nuntil the end",
	"HTTP/1.0 200 OK\r\n\r\nold",
	"HTTP/1.1 204 No Content\r\nX-C: 3\r\n\r\n",
	"HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 201 Created\r\nContent-Length: 2\r\n\r\nok",
	"HTTP/1.1 200 OK\r\nX-D: a\r\nx-d: b\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 404 Not Found\r\nContent-Type: text/plain\r\nContent-Length: 9\r\n\r\nnot found",
	"HTTP/1.1 302 Found\r\nLocation: /r/1\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\nX-T: 1\r\n\r\n",
	"HTTP/1.1 200 OK\r\nContent-Length: 99\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nContent-Length: 3000\r\n\r\n" + strings.Repeat("y", 3000),
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" + strings.Repeat("10\r\n"+strings.Repeat("z", 16)+"\r\n", 300) + "0\r\n\r\n",
	"HTTP/1.1 303 See Other\r\nLocation: http://ADDR/r/3\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 301 Moved\r\nLocation: 7\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 500 Internal Server Error\r\nContent-Length: 4\r\nX-E:   spaced\t \r\n\r\nfail",
	"HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nshort",
	"HTTP/1.1 304 Not Modified\r\nETag: \"x\"\r\n\r\n",
	"HTTP/1.1 200 OK\r\nSet-Cookie: a=1\r\nSet-Cookie: b=2\r\nContent-Length: 1\r\n\r\nx",
}

// http1ClientCorpus are more responses: where the native client and
// net/http differ, and edge cases of the native reading.
var http1ClientCorpus = []string{
	"",
	"HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhel",
	"HTTP/1.1 200 OK\r\nContent-Length: 2",
	"HTTP/1.1 200 OK\nContent-Length: 2\n\nhi",
	"HTTP/1.1 200 OK\r\nNoColon\r\nContent-Length: 2\r\n\r\nhi",
	"HTTP/1.1 2000 OK\r\n\r\n",
	"HTTP/1.1 20 OK\r\n\r\n",
	"HTTP/1.1 200x\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 999 Odd\r\nContent-Length: 0\r\n\r\n",
	"HTTP/9.9 200 OK\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1 200 OK\r\n\r\n",
	"FTP/1.1 200 OK\r\n\r\n",
	"HTTP/1.1 101 Switching Protocols\r\nUpgrade: x\r\n\r\nrest",
	"HTTP/1.1 103 Early Hints\r\nLink: </a>\r\n\r\nHTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\nz",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip, chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: CHUNKED\r\n\r\n3\r\nabc\r\n0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nffffffffffffffffff\r\nabc\r\n0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\nabc\r\n0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\nhello\r\n0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nContent-Length: -1\r\n\r\nto the end",
	"HTTP/1.1 200 OK\r\nContent-Length: +2\r\n\r\nab",
	"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nContent-Length: 3\r\n\r\nabc",
	"HTTP/1.1 200 OK\r\nContent-Length: x\r\n\r\nabc",
	"HTTP/1.1 200 OK\r\n: empty\r\nX-A:\r\nx a: b\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nX-Long: a\r\n b\r\nContent-Length: 0\r\n\r\n",
	"\r\n\r\nHTTP/1.1 200 OK\r\n\r\n",
	"HTTP/1.1 200 OK\r\nX-Ctl: a\x01b\r\nX-Hi: caf\xe9\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1  200 OK\r\n\r\n",
	"HTTP/1.1 \x00malformed \xff" + strings.Repeat("m", 100) + "\r\n\r\n",
	"HTTP/1.1 302 Found\r\nLocation: /r/" + strconv.Itoa(len(http1ClientAgree)+30) + "\r\n\r\n",
	"HTTP/1.1 307 Temporary\r\nLocation: mailto:x\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 308 Permanent\r\nLocation: http://[::1\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 302 Found\r\nLocation: //ADDR/r/2?q=1#frag\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 302 Found\r\nLocation: \r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 302 Found\r\nLocation: http://ADDR/r/a%20b\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.1 200 OK\r\nX-Big: " + strings.Repeat("b", 1<<20+5000) + "\r\n\r\n",
	"HTTP/1.1 200 OK\r\n" + strings.Repeat("X-Many: v\r\n", 200) + "Content-Length: 0\r\n\r\n",
}

// randResponse is a random response: a status line, header fields, and
// a body by Content-Length, chunks, or the end of the stream.
func randResponse(r *rand.Rand) string {
	var b strings.Builder
	protos := []string{"HTTP/1.1", "HTTP/1.0", "HTTP/1.1", "HTTP/2.0"}
	codes := []int{200, 200, 201, 204, 301, 302, 304, 400, 404, 500, 100, 103, 418}
	code := codes[r.Intn(len(codes))]
	fmt.Fprintf(&b, "%s %d Reason\r\n", protos[r.Intn(len(protos))], code)
	if code == 100 || code == 103 {
		b.WriteString("\r\n")
		code = 200
		fmt.Fprintf(&b, "HTTP/1.1 %d OK\r\n", code)
	}
	names := []string{"X-A", "x-b", "Content-Type", "Set-Cookie", "Location", "Connection", "Etag", "Cache-Control"}
	for k := r.Intn(5); k > 0; k-- {
		n := names[r.Intn(len(names))]
		v := fmt.Sprintf("v%d", r.Intn(100))
		if n == "Location" {
			v = "/r/" + strconv.Itoa(r.Intn(40))
		}
		fmt.Fprintf(&b, "%s: %s\r\n", n, v)
	}
	body := strings.Repeat(string(rune('a'+r.Intn(26))), r.Intn(70))
	switch r.Intn(3) {
	case 0:
		fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
	case 1:
		b.WriteString("Transfer-Encoding: chunked\r\n\r\n")
		for len(body) > 0 {
			k := 1 + r.Intn(len(body))
			fmt.Fprintf(&b, "%x\r\n%s\r\n", k, body[:k])
			body = body[k:]
		}
		b.WriteString("0\r\n")
		if r.Intn(3) == 0 {
			b.WriteString("X-Trailer: t\r\n")
		}
		b.WriteString("\r\n")
	default:
		b.WriteString("\r\n" + body)
	}
	return b.String()
}

// http1ClientInputs are the responses of TestHTTP1ClientDiff: the corpus
// and n random ones, every second one mutated.
func http1ClientInputs(n int, seed int64) []string {
	inputs := append(append([]string(nil), http1ClientAgree...), http1ClientCorpus...)
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < n; i++ {
		s := randResponse(r)
		if i%2 == 1 {
			s = string(mutateHTTP(r, []byte(s)))
		}
		inputs = append(inputs, s)
	}
	return inputs
}

// http1RawServer answers a request for /r/<i> with responses[i] (ADDR in
// it replaced by its own address), anything else with a 404, then
// closes the connection. Its address and a stop.
func http1RawServer(t *testing.T, responses []string) (string, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(30 * time.Second))
				br := bufio.NewReader(c)
				line, err := br.ReadString('\n')
				if err != nil {
					return
				}
				for {
					h, err := br.ReadString('\n')
					if err != nil || h == "\r\n" || h == "\n" {
						break
					}
				}
				resp := "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"
				if f := strings.Fields(line); len(f) >= 2 {
					if p, ok := strings.CutPrefix(f[1], "/r/"); ok {
						if i, err := strconv.Atoi(p); err == nil && i >= 0 && i < len(responses) {
							resp = strings.ReplaceAll(responses[i], "ADDR", addr)
						}
					}
				}
				io.WriteString(c, resp)
			}()
		}
	}()
	return addr, func() { l.Close() }
}

// http1ClientGolden is the expected output of the native client for
// http1ClientInputs(http1ClientN, 11), ADDR for the server's address.
var http1ClientGolden = filepath.Join("testdata", "http1_client.txt")

const http1ClientN = 400

// TestHTTP1ClientDiff is the differential test of the native client's
// response reading and redirects in Xo (std/native/llrt/http1.xo
// client_do and parse_response, audit item B10): the same client
// program built with the native backends requests each response of the
// corpus and of random and mutated ones from a server that sends them
// raw; every backend must print testdata/http1_client.txt, which is
// the Go backend's (net/http's) output on every response since the
// native client ports ReadResponse (2026-10-21; before, it was the C
// client's, and the Go backend was held only to http1ClientAgree):
// -golden-update rewrites it from the Go backend.
func TestHTTP1ClientDiff(t *testing.T) {
	xotool.NeedPorts(t)
	if testing.Short() {
		t.Skip("builds an HTTP client with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(http1ClientProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	inputs := http1ClientInputs(http1ClientN, 11)
	addr, stop := http1RawServer(t, inputs)
	defer stop()
	backends := []string{xotool.BackendLLVM, xotool.BackendGo}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	outs := map[string][]string{}
	bins := map[string]string{}
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil {
			t.Fatalf("build (%s): %v", be, err)
		}
		if res.Failed {
			t.Fatalf("build (%s) failed: %v", be, res.Output)
		}
		bins[be] = res.Binaries[xotool.Host()]
		outs[be] = runHTTP1Client(t, be, bins[be], addr, 0, len(inputs))
	}
	if *updateGolden {
		if err := os.WriteFile(http1ClientGolden, []byte(strings.Join(outs[xotool.BackendGo], "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(http1ClientGolden)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	retryHTTP1GoReuse(t, outs[xotool.BackendGo], want, bins[xotool.BackendGo], addr)
	bad := 0
	for _, be := range backends {
		got := outs[be]
		for i := range want {
			g := ""
			if i < len(got) {
				g = got[i]
			}
			if g != want[i] {
				bad++
				if bad <= 15 {
					shown := inputs[i]
					if len(shown) > 300 {
						shown = shown[:300] + fmt.Sprintf("... (%d bytes)", len(inputs[i]))
					}
					if len(g) > 600 {
						g = g[:600] + "..."
					}
					w := want[i]
					if len(w) > 600 {
						w = w[:600] + "..."
					}
					t.Errorf("response %d %q:\n--- %s\n%s\n--- want\n%s", i, shown, be, g, w)
				}
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: %d lines, want %d", be, len(got), len(want))
		}
	}
	if bad > 0 {
		t.Errorf("%d answers differ", bad)
	}
	t.Logf("%d responses, backends %v", len(inputs), backends)
}

// runHTTP1Client runs the client program bin for responses start to n
// at addr and gives its lines, the address written ADDR.
func runHTTP1Client(t *testing.T, be, bin, addr string, start, n int) []string {
	t.Helper()
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Env = append(os.Environ(), "ADDR="+addr, "START="+strconv.Itoa(start), "N="+strconv.Itoa(n))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v\n%s", be, err, stderr.String())
	}
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(out), addr, "ADDR"), "\n"), "\n")
}

// retryHTTP1GoReuse replays, each in a fresh process, the Go backend's
// answers that differ from want, up to three times, and keeps an answer
// that then matches. The golden is each response's answer on a fresh
// connection, and net/http's pool races it: a response that leaves
// bytes past its body (response 437, a 204 with 43 bytes) goes back to
// the idle pool, and when the next request takes the connection before
// the pool's idle read sees the bytes, net/http reads them as that
// request's response (gate deadw-(private commit), response 438, 2026-11-14).
// The native backends get no replay: a reuse bug of theirs shows.
func retryHTTP1GoReuse(t *testing.T, got, want []string, bin, addr string) {
	t.Helper()
	for i := range want {
		if i >= len(got) || got[i] == want[i] {
			continue
		}
		for try := 1; try <= 3; try++ {
			again := runHTTP1Client(t, xotool.BackendGo, bin, addr, i, i+1)
			if len(again) == 1 && again[0] == want[i] {
				t.Logf("go: response %d matched on a fresh connection (try %d); the pooled run gave %q", i, try, got[i])
				got[i] = want[i]
				break
			}
		}
	}
}

// TestHTTPClientRedirectNotUTF8: a redirect whose Location host is not
// UTF-8 (it starts with continuation bytes) reached
// netpolicy.split_hostport through the dial, whose Str slice faulted
// inside the sequence and ended the client (TestHTTPFuzz/client,
// 2026-10-18; decision 0259 decides what such a host becomes). The client
// must answer with an error instead, on each native backend.
func TestHTTPClientRedirectNotUTF8(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an HTTP client with each native backend")
	}
	for _, be := range httpBackends(false) {
		t.Run(be, func(t *testing.T) {
			bin := buildHTTPProgram(t, "client", http1ClientProgram, be)
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			go func() {
				for {
					c, err := l.Accept()
					if err != nil {
						return
					}
					go func() {
						defer c.Close()
						c.SetDeadline(time.Now().Add(10 * time.Second))
						buf := make([]byte, 4096)
						c.Read(buf)
						c.Write([]byte("HTTP/1.1 302 Found\r\nLocation: http://\x82\x82:1/x\r\nContent-Length: 0\r\n\r\n"))
					}()
				}
			}()
			cmd := testtmp.Guard(exec.Command(bin))
			cmd.Env = append(os.Environ(), "ADDR="+l.Addr().String(), "N=2", "XO_LEAKCHECK=1")
			var out, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &stderr
			if err := runWithin(cmd, 2*time.Minute); err != nil {
				t.Fatalf("client: %v\n%s", err, clip(stderr.String()))
			}
			if lines := strings.Count(out.String(), "\n"); lines != 2 {
				t.Errorf("client: %d answers of 2\n%s\n%s", lines, out.String(), clip(stderr.String()))
			}
			t.Logf("%s", out.String())
		})
	}
}

// TestHTTP1ClientGoReuseReplay holds retryHTTP1GoReuse to the race it
// covers: a server that sends a 204's 43 extra bytes 20ms after its head
// makes net/http read them as the next request's response on the pooled
// connection (every time on an idle Mac, 2026-11-15), and the replay in a
// fresh process gives the fresh connection's answer.
func TestHTTP1ClientGoReuseReplay(t *testing.T) {
	xotool.NeedPorts(t)
	if testing.Short() {
		t.Skip("builds an HTTP client")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(http1ClientProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				br := bufio.NewReader(c)
				line, err := br.ReadString('\n')
				if err != nil {
					return
				}
				for {
					h, err := br.ReadString('\n')
					if err != nil || h == "\r\n" {
						break
					}
				}
				if strings.Contains(line, "/r/1 ") {
					io.WriteString(c, "HTTP/1.1 204 No Content\r\n\r\n")
					time.Sleep(20 * time.Millisecond)
					io.WriteString(c, strings.Repeat("y", 43))
					return
				}
				io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
			}()
		}
	}()
	addr := l.Addr().String()
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendGo})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res.Output)
	}
	bin := res.Binaries[xotool.Host()]
	want := []string{"1 204 [] 0 ", "2 200 [Content-Length: 2] 2 6f6b"}
	got := runHTTP1Client(t, xotool.BackendGo, bin, addr, 1, 3)
	if len(got) != 2 || got[0] != want[0] {
		t.Fatalf("pooled run: %q", got)
	}
	if got[1] == want[1] {
		t.Skip("net/http saw the extra bytes before the next request; the race did not happen")
	}
	pooled := got[1]
	all := append([]string{""}, got...)
	retryHTTP1GoReuse(t, all, append([]string{""}, want...), bin, addr)
	if all[2] != want[1] {
		t.Fatalf("replay kept %q (pooled %q), want %q", all[2], pooled, want[1])
	}
	t.Logf("pooled run gave %q; the replay gave the fresh answer", pooled)
}
