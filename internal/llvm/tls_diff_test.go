package llvm_test

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestTLSDiff runs the same HTTPS program (std.md 4.6, decision 0105)
// built with the Go backend and the native backends and requires the
// same observations. Its certificates are made here (a test CA, valid
// for a day), its peers are Go servers and clients on 127.0.0.1:
//
//   - as a client (the program's output): Go servers with an ECDSA and an
//     RSA certificate, TLS 1.2 only and TLS 1.3 only, a certificate of an
//     unknown CA checked against the system's roots, a server offering
//     only RSA key exchange (no shared cipher suite), one requiring a
//     client certificate, one closing the connection, and one answering
//     garbage: statuses, bodies, and the exact error texts;
//   - as a server (probes of this test with crypto/tls and net/http): the
//     certificate chosen per SNI name, ALPN, the TLS versions and cipher
//     suites agreed with a Go client, a TLS 1.1 client refused, plain
//     HTTP answered with net/http's 400, several requests on one
//     connection, and a stop request (SIGTERM) ending serve while an idle
//     TLS connection is open.
//
// The native binaries also run with XO_LEAKCHECK=1: no allocation of
// the runtime may be live at exit.
func TestTLSDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an HTTPS program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	pki := newTestPKI(t)
	peers := startTLSPeers(t, pki)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(tlsDiffProgram(pki, peers)), 0o644); err != nil {
		t.Fatal(err)
	}
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	// the backends build and run in parallel (each build has its own
	// directory, each run its own server port); the group returns when
	// all are done
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
				outs[i] = abruptClose(runTLSDiff(t, res.Binaries[xotool.Host()], pki, be != xotool.BackendGo))
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
			t.Errorf("%s backend differs from the Go backend\n--- go\n%s\n--- %s\n%s", be, want, be, got)
		}
	}
}

// abruptClose makes the two results of the `eof` peer, which closes
// every connection at once, one result: the client reads EOF when the
// close arrives before its ClientHello is sent, and "connection reset by
// peer" when its ClientHello reaches the closed socket first (a race of
// the two hosts' schedulers, on every backend, Go included).
func abruptClose(out string) string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		// Windows words a reset and an abort (WSAECONNRESET,
		// WSAECONNABORTED) as below
		if strings.HasPrefix(l, "eof: ") && (strings.HasSuffix(l, ": EOF)") || strings.HasSuffix(l, "connection reset by peer)") ||
			strings.HasSuffix(l, "forcibly closed by the remote host.)") || strings.HasSuffix(l, "aborted by the software in your host machine.)")) {
			lines[i] = "eof: closed by the peer (EOF or connection reset)"
		}
		// the clientcert peer sends its alert and closes with the
		// client's request unread, which Windows answers with a reset
		// that can overtake the alert (either backend, decision 0197)
		if runtime.GOOS == "windows" && strings.HasPrefix(l, "clientcert: ") &&
			(strings.Contains(l, "certificate required") || strings.HasSuffix(l, ": EOF)") || strings.Contains(l, "forcibly closed") ||
				strings.Contains(l, "aborted by the software")) {
			lines[i] = "clientcert: refused (the alert, or a reset that overtook it)"
		}
	}
	return strings.Join(lines, "\n")
}

// testPKI holds the certificates of TestTLSDiff in PEM.
type testPKI struct {
	caPEM                     []byte
	goodCert, goodKey         []byte // ECDSA, 127.0.0.1 and good.test, chain with the CA
	sniCert, sniKey           []byte // ECDSA, sni.test and *.sni.test
	rsaCert, rsaKey           []byte // RSA 2048, 127.0.0.1
	strayCert, strayKey       []byte // of a CA nobody trusts, 127.0.0.1
	roots                     *x509.CertPool
	goodTLS, rsaTLS, strayTLS tls.Certificate
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	now := time.Now()
	mk := func(tmpl *x509.Certificate, pub, signer any, parent *x509.Certificate) []byte {
		tmpl.NotBefore, tmpl.NotAfter = now.Add(-time.Hour), now.Add(24*time.Hour)
		sn, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		tmpl.SerialNumber = sn
		if parent == nil {
			parent = tmpl
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
		if err != nil {
			t.Fatal(err)
		}
		return der
	}
	ecKey := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	pemOf := func(typ string, der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}) }
	keyPEM := func(k any) []byte {
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatal(err)
		}
		return pemOf("PRIVATE KEY", der)
	}
	caTmpl := func(cn string) *x509.Certificate {
		return &x509.Certificate{Subject: pkix.Name{CommonName: cn}, IsCA: true, BasicConstraintsValid: true,
			KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	}
	leafTmpl := func(cn string, dns []string, ips ...string) *x509.Certificate {
		l := &x509.Certificate{Subject: pkix.Name{CommonName: cn}, DNSNames: dns,
			KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		for _, ip := range ips {
			l.IPAddresses = append(l.IPAddresses, net.ParseIP(ip))
		}
		return l
	}
	p := &testPKI{}
	caKey := ecKey()
	caT := caTmpl("Xo TLS diff CA")
	caDER := mk(caT, &caKey.PublicKey, caKey, nil)
	ca, _ := x509.ParseCertificate(caDER)
	p.caPEM = pemOf("CERTIFICATE", caDER)
	p.roots = x509.NewCertPool()
	p.roots.AddCert(ca)

	gk := ecKey()
	p.goodCert = append(pemOf("CERTIFICATE", mk(leafTmpl("good", []string{"good.test"}, "127.0.0.1"), &gk.PublicKey, caKey, ca)), p.caPEM...)
	p.goodKey = keyPEM(gk)
	sk := ecKey()
	p.sniCert = pemOf("CERTIFICATE", mk(leafTmpl("sni", []string{"sni.test", "*.sni.test"}), &sk.PublicKey, caKey, ca))
	p.sniKey = keyPEM(sk)
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p.rsaCert = pemOf("CERTIFICATE", mk(leafTmpl("rsa", nil, "127.0.0.1"), &rk.PublicKey, caKey, ca))
	p.rsaKey = keyPEM(rk)
	strayCAKey := ecKey()
	strayT := caTmpl("Xo stray CA")
	strayDER := mk(strayT, &strayCAKey.PublicKey, strayCAKey, nil)
	strayCA, _ := x509.ParseCertificate(strayDER)
	xk := ecKey()
	p.strayCert = pemOf("CERTIFICATE", mk(leafTmpl("stray", nil, "127.0.0.1"), &xk.PublicKey, strayCAKey, strayCA))
	p.strayKey = keyPEM(xk)
	for _, kp := range []struct {
		c, k []byte
		out  *tls.Certificate
	}{{p.goodCert, p.goodKey, &p.goodTLS}, {p.rsaCert, p.rsaKey, &p.rsaTLS}, {p.strayCert, p.strayKey, &p.strayTLS}} {
		c, err := tls.X509KeyPair(kp.c, kp.k)
		if err != nil {
			t.Fatal(err)
		}
		*kp.out = c
	}
	return p
}

// startTLSPeers starts the Go servers the program calls: name -> address.
func startTLSPeers(t *testing.T, p *testPKI) map[string]string {
	t.Helper()
	peers := map[string]string{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s %s", r.Method, r.Proto, tls.VersionName(r.TLS.Version))
	})
	serve := func(name string, cfg *tls.Config) {
		l, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: handler, ErrorLog: log.New(io.Discard, "", 0)}
		go srv.Serve(l)
		t.Cleanup(func() { srv.Close() })
		peers[name] = l.Addr().String()
	}
	serve("ecdsa", &tls.Config{Certificates: []tls.Certificate{p.goodTLS}})
	serve("rsa", &tls.Config{Certificates: []tls.Certificate{p.rsaTLS}})
	serve("tls12", &tls.Config{Certificates: []tls.Certificate{p.goodTLS}, MaxVersion: tls.VersionTLS12})
	serve("tls13", &tls.Config{Certificates: []tls.Certificate{p.goodTLS}, MinVersion: tls.VersionTLS13})
	serve("stray", &tls.Config{Certificates: []tls.Certificate{p.strayTLS}})
	serve("rsakex", &tls.Config{Certificates: []tls.Certificate{p.rsaTLS}, MaxVersion: tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_128_GCM_SHA256}})
	serve("clientcert", &tls.Config{Certificates: []tls.Certificate{p.goodTLS}, ClientAuth: tls.RequireAnyClientCert})
	raw := func(name string, f func(net.Conn)) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { l.Close() })
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				go f(c)
			}
		}()
		peers[name] = l.Addr().String()
	}
	raw("eof", func(c net.Conn) { c.Close() })
	raw("garbage", func(c net.Conn) {
		c.Write([]byte("\x00\x01\x02garbage, not TLS\n"))
		time.Sleep(100 * time.Millisecond)
		c.Close()
	})
	return peers
}

// tlsDiffProgram is the Xo program: client calls of each peer, then an
// HTTPS server until a stop request.
func tlsDiffProgram(p *testPKI, peers map[string]string) string {
	var names []string
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(`use std/http

fn describe(e: http.ClientErr) -> Str {
  match e {
    http.ClientErr.InvalidUrl(msg) => "InvalidUrl(${msg})"
    http.ClientErr.Connect(msg) => "Connect(${msg})"
    http.ClientErr.Timeout => "Timeout"
    http.ClientErr.Protocol(msg) => "Protocol(${msg})"
    http.ClientErr.Canceled => "Canceled"
  }
}

fn hide(s: Str, addrs: Map[Str, Str]) -> Str {
  var out = s
  for (name, addr) in addrs {
    out = out.replace(addr, "<${name}>")
  }
  out
}

fn show(os: Os, label: Str, addrs: Map[Str, Str], r: Result[http.Response, http.ClientErr]) {
  match r {
    Ok(resp) => os.stdio.println("${label}: ${resp.status} ${resp.text() ?? "?"}")
    Err(e) => os.stdio.println("${label}: ${hide(describe(e), addrs)}")
  }
}

fn routes() -> http.Router {
  var r = http.router()
  r.handle("GET /hello", fn(req) { http.text(200, "hello ${req.method} ${req.path}") })
  r.handle("POST /echo", fn(req) { http.text(200, "echo ${req.body.to_str() ?? "?"}") })
  r
}

fn main(os: Os) {
`)
	fmt.Fprintf(&b, "  let addrs: Map[Str, Str] = {")
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q: %q", n, peers[n])
	}
	b.WriteString("}\n")
	fmt.Fprintf(&b, "  let ca = %q.bytes()\n", p.caPEM)
	b.WriteString(`  let c = http.client(os.net, timeout: 10s).with_roots(ca).or_fault("roots")
  let system = http.client(os.net, timeout: 10s)
`)
	for _, n := range names {
		fmt.Fprintf(&b, "  show(os, %q, addrs, c.get(\"https://%s/x\"))\n", n, peers[n])
	}
	fmt.Fprintf(&b, "  show(os, \"post\", addrs, c.send(http.new_request(\"POST\", \"https://%s/y\", {}, \"data\".bytes())))\n", peers["ecdsa"])
	fmt.Fprintf(&b, "  show(os, \"system stray\", addrs, system.get(\"https://%s/x\"))\n", peers["stray"])
	fmt.Fprintf(&b, "  let good = http.tls_cert(%q.bytes(), %q.bytes()).or_fault(\"good\")\n", p.goodCert, p.goodKey)
	fmt.Fprintf(&b, "  let sni = http.tls_cert(%q.bytes(), %q.bytes()).or_fault(\"sni\")\n", p.sniCert, p.sniKey)
	b.WriteString(`  let l = os.net.listen("127.0.0.1:0").or_fault("listen")
  os.stdio.println("ADDR ${l.addr()}")
  match http.server(l, routes()).with_tls([good, sni]).serve() {
    Ok(_) => os.stdio.println("served")
    Err(e) => os.stdio.println("serve: ${e}")
  }
}
`)
	return b.String()
}

// runTLSDiff runs the program, probes its server, stops it, and returns
// what it printed and what the probes saw.
func runTLSDiff(t *testing.T, bin string, p *testPKI, native bool) string {
	t.Helper()
	cmd := exec.Command(bin)
	stopAttr(cmd) // a stop request reaches it on Windows too
	if native {
		cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
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
	probeTLSServer(t, &out, addr, p)
	// an idle TLS connection while the server stops
	idle, err := tls.Dial("tcp", addr, &tls.Config{RootCAs: p.roots})
	if err != nil {
		fmt.Fprintf(&out, "idle dial: %v\n", err)
	}
	if err := sendStop(cmd.Process); err != nil {
		t.Fatal(err)
	}
	rest, _ := io.ReadAll(rd)
	out.Write(rest)
	if idle != nil {
		idle.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, err := idle.Read(make([]byte, 1))
		fmt.Fprintf(&out, "idle connection at stop: closed=%v\n", err != nil)
		idle.Close()
	}
	werr := cmd.Wait()
	done = true
	fmt.Fprintf(&out, "exit: %v\n", werr)
	if native {
		m := leakRE.FindStringSubmatch(stderr.String())
		if m == nil || m[1] != "0" {
			t.Errorf("leak check: %q\nstderr:\n%s", m, stderr.String())
		}
	} else if s := strings.TrimSpace(stderr.String()); s != "" {
		fmt.Fprintf(&out, "stderr: %s\n", s)
	}
	return out.String()
}

// probeTLSServer checks the program's HTTPS server with Go clients.
func probeTLSServer(t *testing.T, out *strings.Builder, addr string, p *testPKI) {
	dial := func(label string, cfg *tls.Config) {
		cfg.RootCAs = p.roots
		c, err := tls.Dial("tcp", addr, cfg)
		if err != nil {
			fmt.Fprintf(out, "%s: %v\n", label, err)
			return
		}
		defer c.Close()
		cs := c.ConnectionState()
		fmt.Fprintf(out, "%s: %s %s alpn=%q cert=%s\n", label, tls.VersionName(cs.Version), tls.CipherSuiteName(cs.CipherSuite),
			cs.NegotiatedProtocol, cs.PeerCertificates[0].Subject.CommonName)
	}
	dial("default", &tls.Config{})
	dial("sni good.test", &tls.Config{ServerName: "good.test"})
	dial("sni sni.test", &tls.Config{ServerName: "sni.test"})
	dial("sni a.sni.test", &tls.Config{ServerName: "a.sni.test"})
	dial("sni other.test", &tls.Config{ServerName: "other.test", InsecureSkipVerify: true})
	dial("alpn h2", &tls.Config{NextProtos: []string{"h2", "http/1.1"}})
	dial("tls 1.2", &tls.Config{MaxVersion: tls.VersionTLS12})
	dial("tls 1.2 chacha", &tls.Config{MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256}})
	dial("tls 1.2 cbc", &tls.Config{MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA}})
	dial("tls 1.3", &tls.Config{MinVersion: tls.VersionTLS13})
	dial("tls 1.1", &tls.Config{MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS11})
	dial("p-256 only", &tls.Config{CurvePreferences: []tls.CurveID{tls.CurveP256}})
	// session tickets (decision 0117): a second connection with the
	// client's session cache resumes, on TLS 1.3 and 1.2
	resume := func(label string, version uint16) {
		cfg := &tls.Config{RootCAs: p.roots, MinVersion: version, MaxVersion: version, ClientSessionCache: tls.NewLRUClientSessionCache(4)}
		var got []string
		for range 2 {
			c, err := tls.Dial("tcp", addr, cfg)
			if err != nil {
				fmt.Fprintf(out, "%s: %v\n", label, err)
				return
			}
			// a request and its response: a TLS 1.3 ticket comes after
			// the handshake, and the client reads it with the response
			c.SetDeadline(time.Now().Add(10 * time.Second))
			fmt.Fprintf(c, "GET /hello HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
			io.ReadAll(c)
			got = append(got, fmt.Sprintf("resumed=%v", c.ConnectionState().DidResume))
			c.Close()
		}
		fmt.Fprintf(out, "%s: %s\n", label, strings.Join(got, " "))
	}
	resume("resume tls 1.3", tls.VersionTLS13)
	resume("resume tls 1.2", tls.VersionTLS12)
	// requests over one kept alive connection
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: p.roots}, MaxConnsPerHost: 1}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	for i, path := range []string{"/hello", "/hello?x=1", "/missing"} {
		resp, err := hc.Get("https://" + addr + path)
		if err != nil {
			fmt.Fprintf(out, "get %d: %v\n", i, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Fprintf(out, "get %d: %s %s %q ct=%q\n", i, resp.Status, resp.Proto, body, resp.Header.Get("Content-Type"))
	}
	resp, err := hc.Post("https://"+addr+"/echo", "text/plain", strings.NewReader(strings.Repeat("z", 70000)))
	if err != nil {
		fmt.Fprintf(out, "post: %v\n", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Fprintf(out, "post: %s %d bytes\n", resp.Status, len(body))
	}
	// plain HTTP to the TLS port
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("GET /hello HTTP/1.1\r\nHost: x\r\n\r\n"))
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	raw, _ := io.ReadAll(c)
	c.Close()
	fmt.Fprintf(out, "plain http: %q\n", raw)
}

// TestTLSLeaks builds the http_tls golden with AddressSanitizer and runs
// it with LeakSanitizer on (Linux: macOS's AddressSanitizer has no leak
// detection): Mbed TLS's own allocations (contexts, certificates, roots,
// configurations) must be freed or reachable at exit, which the runtime's
// XO_LEAKCHECK count does not cover.
func TestTLSLeaks(t *testing.T) {
	// the TLS library built with sanitizer flags came from the Go build
	// package (tls.go); xo builds it without them, and this test links
	// it itself: port pending (decision 0215, "Tests not carried over")
	t.Skip("decision 0215: needs a sanitizer build of the TLS library")
	if testing.Short() {
		t.Skip("builds a golden with AddressSanitizer")
	}
	if runtime.GOOS != "linux" {
		t.Skip("LeakSanitizer runs on Linux only")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := filepath.Join("testdata", "golden", "http_tls")
	ir, err := xotool.LLVMIR(dir)
	if err != nil {
		t.Fatal(err)
	}
	ll := filepath.Join(t.TempDir(), "prog.ll")
	if err := os.WriteFile(ll, ir, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := exe(filepath.Join(t.TempDir(), "prog"))
	flags := asanFlags("-O1", "-g", "-fsanitize=address", "-Wno-override-module")
	if err := xotool.CompileC(flags, []string{ll, filepath.Join(rtDir, "xo_rt.c")}, bin); err != nil {
		if strings.Contains(err.Error(), "sanitizer") {
			t.Skipf("no AddressSanitizer: %v", err)
		}
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ASAN_OPTIONS=detect_leaks=1:halt_on_error=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if text := stderr.String(); err != nil || strings.Contains(text, "Sanitizer") {
		if len(text) > 6000 {
			text = text[:6000]
		}
		t.Fatalf("run: %v\n%s", err, text)
	}
	if !strings.Contains(stdout.String(), "done") {
		t.Fatalf("incomplete output:\n%s", stdout.String())
	}
}
