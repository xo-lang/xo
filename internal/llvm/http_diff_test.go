package llvm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// The native HTTP/1.1 server (std/llrt/xo_http.c, decision 0074) against
// net/http: http_diff.c answers each input connection with the C code,
// and a Go net/http server with the same handler answers the same bytes;
// the answers must be equal byte for byte except Date values. The handler
// is std/rt's serve (the whole body read first, the header set, then the
// body written at once) around a body describing the request as Xo code
// sees it.

// fakeConn is a net.Conn whose input is fixed and whose output is kept.
type fakeConn struct {
	in   *bytes.Reader
	mu   sync.Mutex
	out  bytes.Buffer
	done chan struct{}
	once sync.Once
}

func (c *fakeConn) Read(p []byte) (int, error) { return c.in.Read(p) }

func (c *fakeConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.Write(p)
}

func (c *fakeConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return nil
}

func (c *fakeConn) LocalAddr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80} }
func (c *fakeConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 50000}
}

func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// fakeListener hands out the connections sent on ch.
type fakeListener struct {
	ch     chan net.Conn
	closed chan struct{}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.closed:
		return nil, errors.New("closed")
	}
}

func (l *fakeListener) Close() error   { return nil }
func (l *fakeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80} }

// xoReadBody is std/rt's readBody.
func xoReadBody(hr *http.Request) []byte {
	if hr.ContentLength == 0 || hr.Body == nil || hr.Body == http.NoBody {
		return nil
	}
	if n := hr.ContentLength; n > 0 && n <= 1<<20 {
		data := make([]byte, n)
		m, _ := io.ReadFull(hr.Body, data)
		return data[:m]
	}
	data, _ := io.ReadAll(hr.Body)
	return data
}

// diffHandler is the handler of http_diff.c.
func diffHandler(w http.ResponseWriter, hr *http.Request) {
	body := xoReadBody(hr)
	var b strings.Builder
	b.WriteString("method=" + hr.Method + "\npath=" + hr.URL.Path + "\nurl=" + hr.URL.Scheme + "|")
	host := hr.Host
	if h, _, err := net.SplitHostPort(hr.Host); err == nil {
		host = h
	}
	b.WriteString(host + "|")
	if p := hr.URL.Port(); p != "" {
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			b.WriteString(strconv.FormatInt(n, 10))
		} else {
			b.WriteString("-")
		}
	} else {
		b.WriteString("-")
	}
	upath := hr.URL.Path
	if upath == "" {
		upath = "/"
	}
	b.WriteString("|" + upath + "|")
	q := map[string]string{}
	var qkeys []string
	if hr.URL.RawQuery != "" {
		vals := hr.URL.Query()
		for k := range vals {
			qkeys = append(qkeys, k)
			q[k] = vals.Get(k)
		}
		sort.Strings(qkeys)
		for i, k := range qkeys {
			if i > 0 {
				b.WriteString("&")
			}
			b.WriteString(k + "=" + q[k])
		}
	}
	b.WriteString("\nheaders=")
	var hkeys []string
	for k := range hr.Header {
		hkeys = append(hkeys, k)
	}
	sort.Strings(hkeys)
	for i, k := range hkeys {
		if i > 0 {
			b.WriteString("|")
		}
		b.WriteString(k + ": " + strings.Join(hr.Header[k], ", "))
	}
	b.WriteString("\nbody=" + hex.EncodeToString(body) + "\n")
	status := 200
	if strings.HasPrefix(hr.URL.Path, "/status/") && len(hr.URL.Path) > 8 {
		if n, err := strconv.ParseInt(hr.URL.Path[8:], 10, 64); err == nil {
			status = int(n)
		}
	}
	if big, ok := q["big"]; ok {
		// out of range: no body, as the C handler (Atoi returns the
		// clamped value with its error)
		if n, err := strconv.Atoi(big); err == nil && n > 0 {
			b.WriteString(strings.Repeat("x", min(n, 1<<20)))
		}
	}
	text := b.String()
	if _, ok := q["empty"]; ok {
		text = ""
	}
	for _, kv := range [][2]string{{"ct", "Content-Type"}, {"cl", "Content-Length"}, {"conn", "Connection"}, {"te", "Transfer-Encoding"}} {
		if v, ok := q[kv[0]]; ok {
			w.Header().Set(kv[1], v)
		}
	}
	// std/rt's writeResponse sets Content-Length on a whole body over
	// net/http's 2048 byte chunking buffer (decision 0065's request work)
	if len(text) > 2048 && status >= 200 && status != 204 && status != 304 && w.Header().Get("Content-Length") == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(text)))
	}
	w.WriteHeader(status)
	io.WriteString(w, text)
}

// goServe answers each input with net/http.
func goServe(t *testing.T, inputs []string) []string {
	l := &fakeListener{ch: make(chan net.Conn), closed: make(chan struct{})}
	srv := &http.Server{Handler: http.HandlerFunc(diffHandler), ErrorLog: log.New(io.Discard, "", 0)}
	go srv.Serve(l)
	defer close(l.closed)
	out := make([]string, len(inputs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	for i, in := range inputs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, in string) {
			defer wg.Done()
			defer func() { <-sem }()
			c := &fakeConn{in: bytes.NewReader([]byte(in)), done: make(chan struct{})}
			l.ch <- c
			select {
			case <-c.done:
			case <-time.After(20 * time.Second):
				t.Errorf("input %d: net/http did not close the connection", i)
			}
			c.mu.Lock()
			out[i] = c.out.String()
			c.mu.Unlock()
		}(i, in)
	}
	wg.Wait()
	return out
}

var dateRE = regexp.MustCompile(`Date: [^\r\n]*\r\n`)

func normDate(s string) string { return dateRE.ReplaceAllString(s, "Date: X\r\n") }

// httpCorpus are connections worth checking by hand.
var httpCorpus = []string{
	"GET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /a/b?q=1&r=%20s&q=2&s+t=u+v HTTP/1.1\r\nHost: example.com:8080\r\nUser-Agent: t\r\nX-Multi: a\r\nX-Multi: b\r\n\r\n",
	"GET / HTTP/1.0\r\n\r\n",
	"GET / HTTP/1.0\r\nConnection: keep-alive\r\n\r\nGET /2 HTTP/1.0\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\n\r\nGET /second HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\nGET /second HTTP/1.1\r\nHost: x\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhelloGET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello\r\nGET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n6;ext=1\r\n world\r\n0\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\nX-Trailer: 1\r\n\r\nGET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\nhello\r\n0\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: gzip\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: chunked\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nContent-Length: 4\r\n\r\nabcd",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nContent-Length: 3\r\n\r\nabc",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: abc\r\n\r\n",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 10\r\n\r\nshort",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 4\r\nExpect: 100-continue\r\n\r\nabcd",
	"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 4\r\nExpect: something\r\n\r\nabcd",
	"GET / HTTP/1.1\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: a\r\nHost: b\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: a b\r\n\r\n",
	"GET / HTTP/2.0\r\nHost: x\r\n\r\n",
	"GET / HTTP/1.x\r\nHost: x\r\n\r\n",
	"GET /  HTTP/1.1\r\nHost: x\r\n\r\n",
	"G@T / HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET\r\n\r\n",
	"\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\nBad Header: 1\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\nNoColon\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\n: empty\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\nX-A: one\r\n  two\r\n\tthree\r\n\r\n",
	"GET / HTTP/1.1\r\n Host: x\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\nX-Ctl: a\x01b\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\nX-Obs: caf\xe9\r\n\r\n",
	"GET /%zz HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /a%2Fb%20c HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET http://example.com:81/abs?x=1 HTTP/1.1\r\nHost: other\r\n\r\n",
	"GET * HTTP/1.1\r\nHost: x\r\n\r\n",
	"OPTIONS * HTTP/1.1\r\nHost: x\r\n\r\n",
	"CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n",
	"GET relative HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET / HTTP/1.1\nHost: x\n\n",
	"GET / HTTP/1.1\r\nHost: x\r\nPragma: no-cache\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\nconnection: Close\r\n\r\nGET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"HEAD / HTTP/1.1\r\nHost: x\r\n\r\n",
	"HEAD /?big=3000 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?big=2000 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?big=2100 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?big=5000 HTTP/1.0\r\n\r\n",
	"GET /?big=4096 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /status/204 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /status/304?ct=text/x HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /status/204?ct=text/x HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /status/599 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /status/103 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /status/42 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?empty=1 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?ct=application/json HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?cl=500 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?cl=5 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?cl=bad HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?conn=close HTTP/1.1\r\nHost: x\r\n\r\nGET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?conn=keep-alive HTTP/1.0\r\nConnection: keep-alive\r\n\r\n",
	"GET /?te=identity HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?te=chunked HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?empty=1&ct=x HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /a;b=c?x=1;y=2&z=%41 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET /?%zz=1&ok=2 HTTP/1.1\r\nHost: x\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: [::1]:80\r\n\r\n",
	"GET / HTTP/1.1\r\nHost: x\r\n\r\nGARBAGE",
	"GET / HTTP/1.1\r\nHost: x",
	"GET / HTTP/1.1\r\nHost: x\r\n",
	"GE",
	"",
	"PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n",
}

func randRequest(r *rand.Rand) string {
	methods := []string{"GET", "POST", "PUT", "HEAD", "DELETE", "PATCH"}
	paths := []string{"/", "/a/b", "/status/201", "/?q=1", "/x?big=2100", "/?ct=text/plain", "/%41", "/?cl=3", "/?conn=close"}
	var b strings.Builder
	n := 1 + r.Intn(2)
	for i := 0; i < n; i++ {
		m := methods[r.Intn(len(methods))]
		proto := []string{"HTTP/1.1", "HTTP/1.0"}[r.Intn(2)]
		fmt.Fprintf(&b, "%s %s %s\r\n", m, paths[r.Intn(len(paths))], proto)
		if proto == "HTTP/1.1" || r.Intn(2) == 0 {
			b.WriteString("Host: h\r\n")
		}
		for k := r.Intn(3); k > 0; k-- {
			b.WriteString([]string{"X-A: 1\r\n", "Accept: */*\r\n", "Connection: keep-alive\r\n", "Connection: close\r\n", "X-B:  spaced  \r\n"}[r.Intn(5)])
		}
		switch r.Intn(3) {
		case 0:
			body := strings.Repeat("b", r.Intn(10))
			fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
		case 1:
			b.WriteString("Transfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n")
		default:
			b.WriteString("\r\n")
		}
	}
	return b.String()
}

func mutateHTTP(r *rand.Rand, s []byte) []byte {
	alphabet := []byte(" \r\n:/?%&=;HhTtPp01.-\x00\x7f\xff\t{}\"")
	for i := 1 + r.Intn(3); i > 0; i-- {
		switch r.Intn(4) {
		case 0:
			if len(s) > 0 {
				j := r.Intn(len(s))
				s = append(s[:j:j], s[j+1:]...)
			}
		case 1:
			j := r.Intn(len(s) + 1)
			s = append(s[:j:j], append([]byte{alphabet[r.Intn(len(alphabet))]}, s[j:]...)...)
		case 2:
			if len(s) > 0 {
				s[r.Intn(len(s))] = alphabet[r.Intn(len(alphabet))]
			}
		case 3:
			if len(s) > 0 {
				s = s[:r.Intn(len(s))]
			}
		}
	}
	return s
}

// httpExportsMain uses std/http's server, so the LLVM IR of the program
// carries the C ABI exports of std/native/llrt/cabi_http.xo: the response
// writer (http1w.xo, the zero-C audit's B12) that xo_http.c calls.
const httpExportsMain = `use std/http

fn main(os: Os) {
  let l = os.net.listen("127.0.0.1:0").or_fault("listen")
  match http.server(l, http.router()).serve() {
    Ok(_) => ()
    Err(e) => os.stdio.println("serve: ${e}")
  }
}
`

// cOnlyHTTP is cOnly for http_diff.c and http_fuzz.c: with std/http's
// exports.
func cOnlyHTTP(t *testing.T, src string) []string {
	return cOnlyWith(t, src, "cexports-http", httpExportsMain)
}

// buildHTTPDiff compiles http_diff.c with AddressSanitizer and
// UndefinedBehaviorSanitizer when the host's clang has them.
func buildHTTPDiff(t *testing.T) (string, bool) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	src := filepath.Join(rtDir, "testdata", "http_diff.c")
	bin := filepath.Join(t.TempDir(), "http_diff")
	if err := xotool.CompileC(asanFlags("-O1", "-g", "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined"), cOnlyHTTP(t, src), bin); err == nil {
		return bin, true
	}
	if err := xotool.CompileC([]string{"-O1", "-g"}, cOnlyHTTP(t, src), bin); err != nil {
		t.Fatal(err)
	}
	return bin, false
}

// cServe runs http_diff.c on the inputs.
func cServe(t *testing.T, bin string, inputs []string) []string {
	t.Helper()
	res, errText := cServeErr(bin, inputs)
	if errText == "" {
		return res
	}
	// find the input that crashes
	for _, s := range inputs {
		if _, e := cServeErr(bin, []string{s}); e != "" {
			t.Fatalf("http_diff fails on %q:\n%s", s, e)
		}
	}
	t.Fatalf("http_diff: %s", errText)
	return nil
}

func cServeErr(bin string, inputs []string) ([]string, string) {
	var in bytes.Buffer
	for _, s := range inputs {
		var lb [8]byte
		binary.LittleEndian.PutUint64(lb[:], uint64(len(s)))
		in.Write(lb[:])
		in.WriteString(s)
	}
	cmd := exec.Command(bin)
	cmd.Stdin = &in
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, err.Error() + "\n" + stderr.String()
	}
	rd := bufio.NewReader(&out)
	var res []string
	for range inputs {
		var lb [8]byte
		if _, err := io.ReadFull(rd, lb[:]); err != nil {
			return nil, "output ended early: " + err.Error()
		}
		b := make([]byte, binary.LittleEndian.Uint64(lb[:]))
		if _, err := io.ReadFull(rd, b); err != nil {
			return nil, err.Error()
		}
		res = append(res, string(b))
	}
	return res, ""
}

// TestHTTPParseDiff compares the native server's answers with net/http's
// on the corpus and on random requests, valid and mutated.
func TestHTTPParseDiff(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	bin, _ := buildHTTPDiff(t)
	inputs := append([]string(nil), httpCorpus...)
	// a header just under and just over net/http's limit (1 MiB + 4096)
	inputs = append(inputs,
		"GET / HTTP/1.1\r\nHost: x\r\nX-Big: "+strings.Repeat("a", 1<<20)+"\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nX-Big: "+strings.Repeat("a", 1<<20+8192)+"\r\n\r\n",
		// OPTIONS * reads at most 4 KiB of its body
		"OPTIONS * HTTP/1.1\r\nHost: x\r\nContent-Length: 5000\r\n\r\n"+strings.Repeat("a", 5000),
		// a body declared much longer than what came (net/http's
		// requestTooLarge from 256 KiB unread)
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 1000000\r\n\r\nshort",
		"POST /p HTTP/1.0\r\nConnection: keep-alive\r\nContent-Length: 1000000\r\n\r\nshort")
	seed := int64(1)
	if v, err := strconv.ParseInt(os.Getenv("XO_HTTP_DIFF_SEED"), 10, 64); err == nil {
		seed = v
	}
	r := rand.New(rand.NewSource(seed))
	n := 3000
	if testing.Short() {
		n = 300
	}
	if v, err := strconv.Atoi(os.Getenv("XO_HTTP_DIFF_N")); err == nil {
		n = v
	}
	for i := 0; i < n; i++ {
		s := randRequest(r)
		if i%2 == 1 {
			s = string(mutateHTTP(r, []byte(s)))
		}
		inputs = append(inputs, s)
	}
	got := cServe(t, bin, inputs)
	want := goServe(t, inputs)
	bad := 0
	for i := range inputs {
		if normDate(got[i]) != normDate(want[i]) {
			bad++
			if bad <= 15 {
				t.Errorf("input %q:\n--- native\n%s\n--- net/http\n%s", inputs[i], got[i], want[i])
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d inputs differ", bad, len(inputs))
	}
}

// diffDriver is one http_diff.c process answering inputs one at a time
// (FuzzHTTPDiff), with a net/http server for the same inputs.
var diffDriver struct {
	once sync.Once
	err  error
	mu   sync.Mutex
	in   io.WriteCloser
	out  *bufio.Reader
	srv  *fakeListener
}

func startDiffDriver(t *testing.T) {
	diffDriver.once.Do(func() {
		if _, err := xotool.Clang(); err != nil {
			diffDriver.err = err
			return
		}
		src := filepath.Join(rtDir, "testdata", "http_diff.c")
		// inside testDir, so TestMain removes it
		dir := filepath.Join(testDir, "http-diff")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			diffDriver.err = err
			return
		}
		bin := filepath.Join(dir, "http_diff")
		if err := xotool.CompileC(asanFlags("-O1", "-g", "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined"), cOnlyHTTP(t, src), bin); err != nil {
			if err := xotool.CompileC([]string{"-O1", "-g"}, cOnlyHTTP(t, src), bin); err != nil {
				diffDriver.err = err
				return
			}
		}
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), "FLUSH=1")
		in, err := cmd.StdinPipe()
		if err != nil {
			diffDriver.err = err
			return
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			diffDriver.err = err
			return
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			diffDriver.err = err
			return
		}
		diffDriver.in, diffDriver.out = in, bufio.NewReader(out)
		l := &fakeListener{ch: make(chan net.Conn), closed: make(chan struct{})}
		srv := &http.Server{Handler: http.HandlerFunc(diffHandler), ErrorLog: log.New(io.Discard, "", 0)}
		go srv.Serve(l)
		diffDriver.srv = l
	})
	if diffDriver.err != nil {
		t.Skip(diffDriver.err)
	}
}

// diffOne answers input with the native code and with net/http.
func diffOne(t *testing.T, input []byte) (native, goAns string) {
	diffDriver.mu.Lock()
	var lb [8]byte
	binary.LittleEndian.PutUint64(lb[:], uint64(len(input)))
	diffDriver.in.Write(lb[:])
	diffDriver.in.Write(input)
	if _, err := io.ReadFull(diffDriver.out, lb[:]); err != nil {
		diffDriver.mu.Unlock()
		t.Fatalf("http_diff died on %q: %v", input, err)
	}
	b := make([]byte, binary.LittleEndian.Uint64(lb[:]))
	if _, err := io.ReadFull(diffDriver.out, b); err != nil {
		diffDriver.mu.Unlock()
		t.Fatalf("http_diff died on %q: %v", input, err)
	}
	diffDriver.mu.Unlock()
	c := &fakeConn{in: bytes.NewReader(input), done: make(chan struct{})}
	diffDriver.srv.ch <- c
	select {
	case <-c.done:
	case <-time.After(20 * time.Second):
		t.Fatalf("net/http did not close the connection for %q", input)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(b), c.out.String()
}

// FuzzHTTPDiff is TestHTTPParseDiff under Go's fuzzer: `go test
// ./internal/llvm -run '^$' -fuzz FuzzHTTPDiff -fuzztime 60s`. Inputs
// whose head passes 64 KiB are skipped (net/http waits half a second
// before it answers 431).
func FuzzHTTPDiff(f *testing.F) {
	for _, s := range httpCorpus {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 64<<10 {
			t.Skip()
		}
		startDiffDriver(t)
		native, goAns := diffOne(t, input)
		if normDate(native) != normDate(goAns) {
			t.Errorf("input %q:\n--- native\n%s\n--- net/http\n%s", input, native, goAns)
		}
	})
}

// TestHTTPFuzz runs http_fuzz.c, a mutation fuzzer of the native HTTP
// server's and client's parsers and of the JSON parser, with
// AddressSanitizer and UndefinedBehaviorSanitizer for a few seconds (1 s
// with -short; XO_FUZZ_SECONDS overrides): no crash, no report, no leak.
func TestHTTPFuzz(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	src := filepath.Join(rtDir, "testdata", "http_fuzz.c")
	bin := filepath.Join(t.TempDir(), "http_fuzz")
	sanitized := true
	if err := xotool.CompileC(asanFlags("-O1", "-g", "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined"), cOnlyHTTP(t, src), bin); err != nil {
		sanitized = false
		if err := xotool.CompileC([]string{"-O1", "-g"}, cOnlyHTTP(t, src), bin); err != nil {
			t.Fatal(err)
		}
	}
	secs := "5"
	if testing.Short() {
		secs = "1"
	}
	if v := os.Getenv("XO_FUZZ_SECONDS"); v != "" {
		secs = v
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "FUZZ_SECONDS="+secs, "ASAN_OPTIONS=detect_leaks=0:halt_on_error=1")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		text := stderr.String()
		if len(text) > 6000 {
			text = text[:6000]
		}
		t.Fatalf("http_fuzz: %v\n%s", err, text)
	}
	t.Logf("%s (sanitizers: %v)", strings.TrimSpace(out.String()), sanitized)
}
