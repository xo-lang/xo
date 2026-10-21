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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// The native HTTP/1.1 server (std/native/llrt/http1.xo and http1w.xo,
// the zero-C audit's B28) against net/http itself: http1DiffProgram
// (http1_diff_test.go) built with a native backend answers each input on
// a connection of its own, and a Go net/http server with the same
// handler (diffHandler) answers the same bytes in process; the answers
// must be equal byte for byte except Date values. The handler is std/rt's
// serve (the whole body read first, the header set, then the body written
// at once) around a body describing the request as Xo code sees it.
// TestHTTP1ParseDiff compares the same program across backends; this
// compares it with net/http directly (no std/rt in between).

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

// urlText is std's rule (decision 0259) for a URL path, host, or query
// part: valid UTF-8 as it is, each byte of an invalid sequence a percent
// escape (scan.xo url_text_at).
func urlText(s string) string { return fixUTF8(s, true) }

// validText is std's rule for other outside text: each byte of an
// invalid sequence U+FFFD (scan.xo text_at).
func validText(s string) string { return fixUTF8(s, false) }

func fixUTF8(s string, esc bool) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			if esc {
				fmt.Fprintf(&b, "%%%02X", s[i])
			} else {
				b.WriteString("\uFFFD")
			}
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

// diffHandler is http1DiffProgram's handler in Go, with the native
// server's text rules (decision 0259) applied to what it reports.
func diffHandler(w http.ResponseWriter, hr *http.Request) {
	body := xoReadBody(hr)
	var b strings.Builder
	b.WriteString("method=" + hr.Method + "\npath=" + urlText(hr.URL.Path) + "\nurl=" + hr.URL.Scheme + "|")
	host := hr.Host
	if h, _, err := net.SplitHostPort(hr.Host); err == nil {
		host = h
	}
	b.WriteString(urlText(host) + "|")
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
	b.WriteString("|" + urlText(upath) + "|")
	q := map[string]string{}
	var qkeys []string
	if hr.URL.RawQuery != "" {
		vals := hr.URL.Query()
		for k := range vals {
			// the escaped key is the one sorted, as the native server
			// sorts its query's keys (decision 0259)
			ek := urlText(k)
			qkeys = append(qkeys, ek)
			q[ek] = urlText(vals.Get(k))
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
		joined := validText(strings.Join(hr.Header[k], ", "))
		b.WriteString(validText(k) + ": " + joined)
		// every field of a repeated name, as http1DiffProgram prints it
		// (req.header_values, decision 0288; 3065af16 added it there)
		if vs := hr.Header[k]; len(vs) != 1 || validText(vs[0]) != joined {
			fs := make([]string, len(vs))
			for i, v := range vs {
				fs[i] = validText(v)
			}
			b.WriteString("|" + validText(k) + " fields: " + strings.Join(fs, " ;; "))
		}
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

// httpBuilds are the programs of these tests, built once per test binary
// (key: program name and backend).
var httpBuilds sync.Map

type httpBuild struct {
	once sync.Once
	bin  string
	err  error
}

// buildHTTPProgram builds src with the backend once, in testDir.
func buildHTTPProgram(t testing.TB, name, src, backend string) string {
	t.Helper()
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	v, _ := httpBuilds.LoadOrStore(name+"/"+backend, &httpBuild{})
	b := v.(*httpBuild)
	b.once.Do(func() {
		dir := filepath.Join(testDir, "http-"+name+"-"+backend)
		if b.err = os.MkdirAll(dir, 0o755); b.err != nil {
			return
		}
		if b.err = os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); b.err != nil {
			return
		}
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: backend})
		if err != nil {
			b.err = err
			return
		}
		if res.Failed {
			b.err = fmt.Errorf("build failed: %v", res.Output)
			return
		}
		b.bin = res.Binaries[xotool.Host()]
	})
	if b.err != nil {
		t.Fatalf("%s (%s): %v", name, backend, b.err)
	}
	return b.bin
}

// httpBackends are the native backends these tests build with: the
// host's fastest alone (arm64 on arm64, else LLVM) when one is enough.
func httpBackends(all bool) []string {
	if runtime.GOARCH != "arm64" {
		return []string{xotool.BackendLLVM}
	}
	if all {
		return []string{xotool.BackendLLVM, xotool.BackendArm64}
	}
	return []string{xotool.BackendArm64}
}

// nativeSend is http1Send with its retries: an answer cut by a reset is
// sent again (whether the reset overtakes the answer is timing), and a
// reset left at the end is dropped (net/http in process has none).
func nativeSend(addr, input string) string {
	var out string
	for try := 0; try < 4; try++ {
		out = http1Send(addr, input)
		if !strings.HasSuffix(out, "<reset>") {
			return out
		}
	}
	return strings.TrimSuffix(out, "<reset>")
}

// httpParseDiffInputs are TestHTTPParseDiff's inputs: the corpus, the
// header limits, bodies declared much longer than what came, and n
// random requests, every second one mutated.
func httpParseDiffInputs(n int, seed int64) []string {
	inputs := append([]string(nil), httpCorpus...)
	inputs = append(inputs,
		// a header just under and just over net/http's limit (1 MiB + 4096)
		"GET / HTTP/1.1\r\nHost: x\r\nX-Big: "+strings.Repeat("a", 1<<20)+"\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: x\r\nX-Big: "+strings.Repeat("a", 1<<20+8192)+"\r\n\r\n",
		// OPTIONS * reads at most 4 KiB of its body
		"OPTIONS * HTTP/1.1\r\nHost: x\r\nContent-Length: 5000\r\n\r\n"+strings.Repeat("a", 5000),
		// a body declared much longer than what came (net/http's
		// requestTooLarge from 256 KiB unread): the native server closes
		// after it as net/http does (gotchas/http-short-body-close-differs.md)
		"POST /p HTTP/1.1\r\nHost: x\r\nContent-Length: 1000000\r\n\r\nshort",
		"POST /p HTTP/1.0\r\nConnection: keep-alive\r\nContent-Length: 1000000\r\n\r\nshort")
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

// TestHTTPParseDiff compares the native server's answers with net/http's
// on the corpus and on random requests, valid and mutated: LLVM and arm64
// (on arm64), 3000 random requests (XO_HTTP_DIFF_N, XO_HTTP_DIFF_SEED).
func TestHTTPParseDiff(t *testing.T) {
	xotool.NeedPorts(t)
	if testing.Short() {
		t.Skip("builds an HTTP server with the native backends")
	}
	n := 3000
	if v, err := strconv.Atoi(os.Getenv("XO_HTTP_DIFF_N")); err == nil {
		n = v
	}
	seed := int64(1)
	if v, err := strconv.ParseInt(os.Getenv("XO_HTTP_DIFF_SEED"), 10, 64); err == nil {
		seed = v
	}
	inputs := httpParseDiffInputs(n, seed)
	want := goServe(t, inputs)
	for _, be := range httpBackends(true) {
		t.Run(be, func(t *testing.T) {
			addr, stop := http1Server(t, buildHTTPProgram(t, "parse", http1DiffProgram, be))
			defer stop()
			got := make([]string, len(inputs))
			var wg sync.WaitGroup
			sem := make(chan struct{}, 16)
			for i, in := range inputs {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					got[i] = nativeSend(addr, in)
				}()
			}
			wg.Wait()
			bad := 0
			for i, in := range inputs {
				if normDate(got[i]) != normDate(want[i]) {
					bad++
					if bad <= 15 {
						shown := in
						if len(shown) > 300 {
							shown = shown[:300] + fmt.Sprintf("... (%d bytes)", len(in))
						}
						t.Errorf("input %q:\n--- native\n%s\n--- net/http\n%s", shown, clip(got[i]), clip(want[i]))
					}
				}
			}
			if bad > 0 {
				t.Errorf("%d of %d inputs differ", bad, len(inputs))
			}
			t.Logf("%d inputs", len(inputs))
		})
	}
}

func clip(s string) string {
	if len(s) > 2000 {
		return s[:2000] + "..."
	}
	return s
}

// FuzzHTTPDiff is TestHTTPParseDiff under Go's fuzzer: `go test
// ./internal/llvm -run '^$' -fuzz FuzzHTTPDiff -fuzztime 60s`. Inputs
// whose head passes 64 KiB are skipped (net/http waits half a second
// before it answers 431). Without -fuzz it runs the seed corpus (not
// with -short: it builds the server).
func FuzzHTTPDiff(f *testing.F) {
	for _, s := range httpCorpus {
		f.Add([]byte(s))
	}
	if testing.Short() {
		f.Skip("builds an HTTP server with a native backend")
	}
	// the servers live for the whole fuzz test, f's: started in the first
	// input's t, the native one was killed by that input's cleanup (its
	// lifeline), and every later input dialed a dead address for a minute
	// (2026-10-16)
	bin := buildHTTPProgram(f, "parse", http1DiffProgram, httpBackends(false)[0])
	addr, stop := http1Server(f, bin)
	f.Cleanup(stop)
	l := &fakeListener{ch: make(chan net.Conn), closed: make(chan struct{})}
	srv := &http.Server{Handler: http.HandlerFunc(diffHandler), ErrorLog: log.New(io.Discard, "", 0)}
	go srv.Serve(l)
	f.Cleanup(func() {
		// fakeListener's Close is a no-op: its closed channel ends Accept,
		// so Serve returns and Close does not wait forever
		close(l.closed)
		srv.Close()
	})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 64<<10 {
			t.Skip()
		}
		native := nativeSend(addr, string(input))
		if strings.HasPrefix(native, "dial: ") {
			t.Fatalf("native server: %s", native)
		}
		c := &fakeConn{in: bytes.NewReader(input), done: make(chan struct{})}
		l.ch <- c
		select {
		case <-c.done:
		case <-time.After(20 * time.Second):
			t.Fatalf("net/http did not close the connection for %q", input)
		}
		c.mu.Lock()
		goAns := c.out.String()
		c.mu.Unlock()
		if normDate(native) != normDate(goAns) {
			t.Errorf("input %q:\n--- native\n%s\n--- net/http\n%s", input, native, goAns)
		}
	})
}

// httpFuzzTLSProgram is http1DiffProgram's handler behind TLS (HTTP/2
// by ALPN): its certificate is the test PKI's.
func httpFuzzTLSProgram(p *testPKI) string {
	src := strings.Replace(http1DiffProgram, `  match http.server(l, r).serve() {`,
		fmt.Sprintf("  let cert = http.tls_cert(%q.bytes(), %q.bytes()).or_fault(\"cert\")\n  match http.server(l, r).with_tls([cert]).serve() {", p.goodCert, p.goodKey), 1)
	return src
}

// httpFuzzJSONProgram decodes each line of standard input (hex) as each of
// a few types and prints how many decoded.
const httpFuzzJSONProgram = `use std/json

struct Inner {
  k: List[Int]
  s: Str?
  f: Float = 0.5
} derive Json

struct Outer {
  a: List[Inner]
  b: Map[Str, Bool]
  c: (Int, Str)
  d: Duration?
  e: Bytes?
} derive Json

fn main(os: Os) {
  let all = os.stdio.read_all().or_fault("stdin")
  var ok = 0
  var n = 0
  for line in all.lines() {
    let data = Bytes.from_hex(line) ?? b""
    n += 1
    if json.decode[Outer](data) is Ok(_) {
      ok += 1
    }
    if json.decode[Map[Str, List[Float]]](data) is Ok(_) {
      ok += 1
    }
    if json.decode[List[Str]](data) is Ok(_) {
      ok += 1
    }
    if json.decode[Inner](data) is Ok(_) {
      ok += 1
    }
  }
  os.stdio.println("json ${n} inputs ${ok} decoded")
}
`

// httpFuzzSeeds are http_fuzz.c's seeds and dictionary (the C fuzzer of
// the deleted C parsers): requests, responses, JSON.
var httpFuzzSeeds = []string{
	"GET / HTTP/1.1\r\nHost: x\r\n\r\n",
	"POST /p?a=1&b=%20&c HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello",
	"POST /p HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\nT: 1\r\n\r\n",
	"GET /a HTTP/1.0\r\nConnection: keep-alive\r\n\r\nGET /b HTTP/1.1\r\nHost: y\r\n\r\n",
	"GET http://h:80/x?y HTTP/1.1\r\nHost: z\r\nX: a\r\n b\r\n\r\n",
	"OPTIONS * HTTP/1.1\r\nHost: x\r\nExpect: 100-continue\r\nContent-Length: 2\r\n\r\nab",
}

var httpFuzzResponses = []string{
	"HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nabc",
	"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n",
	"HTTP/1.1 100 Continue\r\n\r\nHTTP/1.0 204 No Content\r\nX-A: 1\r\nX-A: 2\r\n\r\n",
	"HTTP/1.1 301 Moved\r\nLocation: /r/x\r\nContent-Length: 0\r\n\r\n",
	"HTTP/1.0 200 OK\r\n\r\nuntil close",
}

var httpFuzzJSON = []string{
	`{"a": [{"k": [1, 2, 3], "s": "x\u00e9\ud83d\ude00", "f": 2.5e3}], "b": {"x": true, "y": false}, "c": [1, "two"], "d": "1.5s", "e": "aGk="}`,
	`{"a": [], "b": {}, "c": [-0, ""], "d": null}`,
	`["\"", "12345678901234567890", "1e400", "\n"]`,
	`{"k": [9223372036854775807, -9223372036854775808], "s": null}`,
	`{"x": [1.5, -2e-3, 0], "y": [[[]]]}`,
}

var httpFuzzDict = []string{"\r\n", "\n", " ", ":", "Content-Length: ", "Transfer-Encoding: chunked\r\n", "0\r\n\r\n", "Host: h\r\n",
	"HTTP/1.1", "HTTP/1.0", "%", "%2", "?", "&", "=", ";", "Expect: 100-continue\r\n", "Connection: close\r\n",
	"{", "}", "[", "]", "\"", "\\u", ",", "-1", "18446744073709551616", "ffffffffffffffffff\r\n", "\x00", "\xff"}

// fuzzMutate is http_fuzz.c's mutation: up to six edits, each a range
// deleted, a byte set, a dictionary word or random byte inserted, a range
// repeated, or the input cut.
func fuzzMutate(r *rand.Rand, s []byte) []byte {
	for k := 1 + r.Intn(6); k > 0; k-- {
		at := 0
		if len(s) > 0 {
			at = r.Intn(len(s) + 1)
		}
		switch r.Intn(7) {
		case 0:
			if len(s) > 0 {
				end := min(len(s), at+1+r.Intn(8))
				if at < end {
					s = append(s[:at:at], s[end:]...)
				}
			}
		case 1:
			if len(s) > 0 {
				s[r.Intn(len(s))] = byte(r.Intn(256))
			}
		case 2, 3:
			w := httpFuzzDict[r.Intn(len(httpFuzzDict))]
			s = append(s[:at:at], append([]byte(w), s[at:]...)...)
		case 4:
			s = append(s[:at:at], append([]byte{byte(r.Intn(256))}, s[at:]...)...)
		case 5:
			if len(s) > 0 && at < len(s) {
				end := min(len(s), at+1+r.Intn(16))
				rep := bytes.Repeat(s[at:end], 1+r.Intn(4))
				s = append(s[:at:at], append(rep, s[at:]...)...)
			}
		case 6:
			if len(s) > 0 {
				s = s[:r.Intn(len(s))]
			}
		}
	}
	if len(s) > 1<<16 {
		s = s[:1<<16]
	}
	return s
}

// h2FuzzFrames is a valid request's frames after the preface (SETTINGS,
// a request with a body, a WINDOW_UPDATE, PING, PRIORITY, a second request
// over CONTINUATION, a reset): the HTTP/2 seed.
func h2FuzzFrames() []byte {
	var b bytes.Buffer
	fr := func(typ, flags byte, sid uint32, payload []byte) {
		hd := make([]byte, 9)
		hd[0], hd[1], hd[2] = byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload))
		hd[3], hd[4] = typ, flags
		binary.BigEndian.PutUint32(hd[5:], sid)
		b.Write(hd)
		b.Write(payload)
	}
	hb := hpackLit(h2Req("POST", "/a?q=1", "content-length", "3", "x-a", "b")...)
	fr(1, 4, 1, hb)
	fr(0, 1, 1, []byte("abc"))
	fr(8, 0, 0, []byte{0, 0, 0x10, 0})
	fr(6, 0, 0, []byte("12345678"))
	fr(2, 0, 3, []byte{0, 0, 0, 1, 16})
	hb2 := hpackLit(h2Req("GET", "/status/204")...)
	fr(1, 0, 3, hb2[:len(hb2)/2])
	fr(9, 4, 3, hb2[len(hb2)/2:])
	fr(3, 0, 5, []byte{0, 0, 0, 8})
	return b.Bytes()
}

// TestHTTPFuzz mutation fuzzes the native std/http parsers in Xo for a
// few seconds each (XO_FUZZ_SECONDS, default 5): the HTTP/1.1 server's
// request reading (http1.xo) over TCP, the HTTP/2 server's frames and
// HPACK (h2srv.xo, hpack.xo) over TLS, the client's response reading
// (hclient.xo, http1.xo) from a Go server sending mutated responses, and
// std/json's decoding (json.xo) of mutated documents. Each program runs
// with XO_LEAKCHECK=1: no fault, a server still answering at the end, a
// clean exit (servers stopped as by SIGTERM) with nothing live. The C
// fuzzer it replaces ran the deleted C parsers under AddressSanitizer;
// these are Xo (bounds checked), so the checks are a fault, a hang, and a
// leak.
func TestHTTPFuzz(t *testing.T) {
	xotool.NeedPorts(t)
	if testing.Short() {
		t.Skip("builds std/http programs with a native backend")
	}
	secs := 5.0
	if v, err := strconv.ParseFloat(os.Getenv("XO_FUZZ_SECONDS"), 64); err == nil {
		secs = v
	}
	dur := time.Duration(secs * float64(time.Second))
	be := httpBackends(false)[0]
	seed := time.Now().UnixNano()
	if v, err := strconv.ParseInt(os.Getenv("FUZZ_SEED"), 10, 64); err == nil {
		seed = v
	}
	t.Logf("seed %d (FUZZ_SEED), %v each, %s", seed, dur, be)
	t.Run("server", func(t *testing.T) {
		bin := buildHTTPProgram(t, "parse", http1DiffProgram, be)
		s := startFuzzServer(t, bin)
		corpus := append(append([]string(nil), httpCorpus...), httpFuzzSeeds...)
		// XO_FUZZ_REPLAY: a file of Go quoted inputs, one per line, sent
		// one at a time with the server's health checked after each
		if rp := os.Getenv("XO_FUZZ_REPLAY"); rp != "" {
			data, _ := os.ReadFile(rp)
			for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				in, _ := strconv.Unquote(line)
				got := http1Send(s.addr, in)
				if h := http1Send(s.addr, "GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"); !strings.HasPrefix(h, "HTTP/1.1 200 ") {
					t.Fatalf("dead after line %d: %q answered %q; health %q", i, in, clip(got), clip(h))
				}
			}
			t.Fatal("replay: alive")
		}
		var dialErr, empty atomic.Int64
		n := fuzzFor(dur, 8, func(w int) {
			rr := rand.New(rand.NewSource(seed + int64(w)))
			in := []byte(corpus[rr.Intn(len(corpus))])
			if rr.Intn(3) == 0 {
				in = []byte(randRequest(rr))
			}
			out := http1Send(s.addr, string(fuzzMutate(rr, in)))
			if strings.HasPrefix(out, "dial: ") {
				dialErr.Add(1)
			} else if out == "" {
				empty.Add(1)
			}
		})
		t.Logf("dial errors %d, empty answers %d", dialErr.Load(), empty.Load())
		if got := http1Send(s.addr, "GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"); !strings.HasPrefix(got, "HTTP/1.1 200 ") {
			t.Errorf("after %d inputs the server answers %q", n, clip(got))
		}
		s.stop(t)
		t.Logf("%d inputs", n)
	})
	t.Run("http2", func(t *testing.T) {
		p := newTestPKI(t)
		bin := buildHTTPProgram(t, "tls", httpFuzzTLSProgram(p), be)
		s := startFuzzServer(t, bin)
		frames := h2FuzzFrames()
		// at most 3000 connections: one the server closes first leaves
		// its side in TIME_WAIT, and loopback's ports run out near 5000
		var started, conns atomic.Int64
		fuzzFor(dur, 8, func(w int) {
			if started.Add(1) > 3000 {
				time.Sleep(10 * time.Millisecond)
				return
			}
			rr := rand.New(rand.NewSource(seed + 100 + int64(w)))
			h, err := h2Dial(s.addr, p)
			if err != nil {
				return
			}
			conns.Add(1)
			defer func() {
				// a reset at the end leaves no TIME_WAIT behind
				// (thousands of connections run here)
				h.c.NetConn().(*net.TCPConn).SetLinger(0)
				h.c.Close()
			}()
			h.write(fuzzMutate(rr, append([]byte(nil), frames...)))
			// what the server answers, until it closes or goes quiet
			h.c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			io.Copy(io.Discard, h.c)
		})
		var h *h2conn
		var err error
		for try := 0; try < 300; try++ {
			if h, err = h2Dial(s.addr, p); err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond) // ports in TIME_WAIT
		}
		n := int(conns.Load())
		if err != nil {
			t.Fatalf("after %d inputs: %v", n, err)
		}
		h.headers(1, true, h2Req("GET", "/")...)
		h.c.SetReadDeadline(time.Now().Add(10 * time.Second))
		ok := false
		for !ok {
			typ, _, sid, payload, err := h.read()
			if err != nil {
				break
			}
			ok = typ == 1 && sid == 1 && h2Status(payload) == "200"
		}
		h.c.Close()
		if !ok {
			t.Errorf("after %d inputs the server does not answer over HTTP/2", n)
		}
		s.stop(t)
		t.Logf("%d connections", n)
	})
	t.Run("client", func(t *testing.T) {
		bin := buildHTTPProgram(t, "client", http1ClientProgram, be)
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		var mu sync.Mutex
		rr := rand.New(rand.NewSource(seed + 200))
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				mu.Lock()
				resp := fuzzMutate(rr, []byte(httpFuzzResponses[rr.Intn(len(httpFuzzResponses))]))
				mu.Unlock()
				go func() {
					defer c.Close()
					c.SetDeadline(time.Now().Add(10 * time.Second))
					// the request's head, then the response
					buf := make([]byte, 4096)
					c.Read(buf)
					c.Write(resp)
				}()
			}
		}()
		n := int(secs * 200)
		cmd := exec.Command(bin)
		testtmp.Lifeline(t, cmd, 30*time.Minute) // no orphan if the test is killed
		cmd.Env = append(os.Environ(), "ADDR="+l.Addr().String(), fmt.Sprintf("N=%d", n), "XO_LEAKCHECK=1")
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := runWithin(cmd, dur+2*time.Minute); err != nil {
			t.Fatalf("client: %v\n%s", err, clip(stderr.String()))
		}
		if lines := strings.Count(out.String(), "\n"); lines != n {
			t.Errorf("client: %d answers of %d\n%s", lines, n, clip(stderr.String()))
		}
		t.Logf("%d responses", n)
	})
	t.Run("json", func(t *testing.T) {
		bin := buildHTTPProgram(t, "json", httpFuzzJSONProgram, be)
		rr := rand.New(rand.NewSource(seed + 300))
		var in bytes.Buffer
		n := int(secs * 4000)
		for i := 0; i < n; i++ {
			doc := []byte(httpFuzzJSON[rr.Intn(len(httpFuzzJSON))])
			if i%8 != 0 {
				doc = fuzzMutate(rr, doc)
			}
			in.WriteString(hex.EncodeToString(doc) + "\n")
		}
		cmd := exec.Command(bin)
		testtmp.Lifeline(t, cmd, 30*time.Minute) // no orphan if the test is killed
		cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
		cmd.Stdin = &in
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := runWithin(cmd, 2*time.Minute); err != nil {
			t.Fatalf("json: %v\n%s", err, clip(stderr.String()))
		}
		if !strings.HasPrefix(out.String(), fmt.Sprintf("json %d inputs ", n)) {
			t.Errorf("json: %q", out.String())
		}
		t.Logf("%s", strings.TrimSpace(out.String()))
	})
}

// fuzzServer is a server program run with XO_LEAKCHECK=1.
type fuzzServer struct {
	cmd    *exec.Cmd
	addr   string
	stderr *bytes.Buffer
	done   chan error
}

func startFuzzServer(t *testing.T, bin string) *fuzzServer {
	t.Helper()
	cmd := exec.Command(bin)
	testtmp.Lifeline(t, cmd, 30*time.Minute) // no orphan if the test is killed
	stopAttr(cmd)
	cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	s := &fuzzServer{cmd: cmd, stderr: &bytes.Buffer{}, done: make(chan error, 1)}
	life := &serverLife{done: make(chan struct{})}
	cmd.Stderr = s.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	rd := bufio.NewReader(stdout)
	line, err := rd.ReadString('\n')
	addr, ok := strings.CutPrefix(strings.TrimSpace(line), "ADDR ")
	if err != nil || !ok {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("%s: %q %v\n%s", bin, line, err, s.stderr.String())
	}
	s.addr = addr
	servers.Store(addr, life)
	go func() {
		io.Copy(io.Discard, rd)
		err := cmd.Wait()
		life.err = err
		close(life.done)
		s.done <- err
	}()
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
		}
	})
	return s
}

// stop stops the server as SIGTERM does and requires a clean exit with
// nothing live (XO_LEAKCHECK=1) and no fault.
func (s *fuzzServer) stop(t *testing.T) {
	t.Helper()
	if err := sendStop(s.cmd.Process); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-s.done:
		if err != nil {
			t.Errorf("server exit: %v\n%s", err, clip(s.stderr.String()))
		}
	case <-time.After(time.Minute):
		s.cmd.Process.Kill()
		t.Errorf("server did not stop\n%s", clip(s.stderr.String()))
	}
	// a fault the server did not isolate ("fault: ..." at the start of a
	// line), or memory live at exit; isolated handler faults are logged
	// ("ERROR handler fault") and expected
	e := s.stderr.String()
	if strings.HasPrefix(e, "fault:") || strings.Contains(e, "\nfault:") || strings.Contains(e, "still live") {
		t.Errorf("server stderr:\n%s", clip(s.stderr.String()))
	}
}

// fuzzFor runs f on w workers until d passes; the number of runs.
func fuzzFor(d time.Duration, workers int, f func(w int)) int {
	var mu sync.Mutex
	n := 0
	end := time.Now().Add(d)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; time.Now().Before(end); i++ {
				f(w*1000003 + i)
				mu.Lock()
				n++
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return n
}

// runWithin runs cmd, killing it after d.
func runWithin(cmd *exec.Cmd, d time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	timer := time.AfterFunc(d, func() { cmd.Process.Kill() })
	defer timer.Stop()
	return cmd.Wait()
}
