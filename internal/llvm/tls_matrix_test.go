package llvm

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
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

// matrixCert is a test certificate of the interop matrix: one per key
// type, all signed by one ECDSA CA, valid for 127.0.0.1.
type matrixCert struct {
	name       string
	cert, key  []byte // PEM (the key PKCS #8)
	tls        tls.Certificate
	ecdsaSuite bool // TLS 1.2's ECDHE_ECDSA suites (else ECDHE_RSA)
	goOnly     bool // only Go's servers have it
}

func newMatrixPKI(t *testing.T) ([]byte, *x509.CertPool, []matrixCert) {
	t.Helper()
	now := time.Now()
	pemOf := func(typ string, der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}) }
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caT := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Xo matrix CA"}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour)}
	caDER, err := x509.CreateCertificate(rand.Reader, caT, caT, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	p256, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p521, _ := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	_, ed, _ := ed25519.GenerateKey(rand.Reader)
	var out []matrixCert
	for i, k := range []struct {
		name  string
		priv  any
		pub   any
		ecdsa bool
	}{{"rsa", rk, &rk.PublicKey, false}, {"p256", p256, &p256.PublicKey, true}, {"p384", p384, &p384.PublicKey, true}, {"ed25519", ed, ed.Public(), true}, {"p521", p521, &p521.PublicKey, true}} {
		lt := &x509.Certificate{SerialNumber: big.NewInt(int64(10 + i)), Subject: pkix.Name{CommonName: k.name}, NotBefore: now.Add(-time.Hour),
			NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		der, err := x509.CreateCertificate(rand.Reader, lt, ca, k.pub, caKey)
		if err != nil {
			t.Fatal(err)
		}
		kd, err := x509.MarshalPKCS8PrivateKey(k.priv)
		if err != nil {
			t.Fatal(err)
		}
		mc := matrixCert{name: k.name, cert: pemOf("CERTIFICATE", der), key: pemOf("PRIVATE KEY", kd), ecdsaSuite: k.ecdsa, goOnly: false}
		if mc.tls, err = tls.X509KeyPair(mc.cert, mc.key); err != nil {
			t.Fatal(err)
		}
		out = append(out, mc)
	}
	return pemOf("CERTIFICATE", caDER), roots, out
}

// matrixSuites12 is TLS 1.2's suites for a certificate's key: GCM,
// ChaCha20-Poly1305, and CBC.
func matrixSuites12(ecdsaKey bool) []uint16 {
	if ecdsaKey {
		return []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA}
	}
	return []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA}
}

var matrixGroups = []struct {
	name string
	id   tls.CurveID
}{{"x25519", tls.X25519}, {"p256", tls.CurveP256}, {"p384", tls.CurveP384}, {"p521", tls.CurveP521}}

// matrixClientGroups are the groups of the Go servers the Xo client
// calls (all of matrixGroups).
var matrixClientGroups = matrixGroups

// matrixHybrids are Go 1.26's hybrids other than X25519MLKEM768 (its
// default), TLS 1.3 only.
var matrixHybrids = []struct {
	name string
	id   tls.CurveID
}{{"secp256r1mlkem768", tls.SecP256r1MLKEM768}, {"secp384r1mlkem1024", tls.SecP384r1MLKEM1024}}

// matrixState is what one side of a connection saw.
func matrixState(cs tls.ConnectionState) string {
	alpn := cs.NegotiatedProtocol
	if alpn == "" {
		alpn = "-"
	}
	return fmt.Sprintf("%s %s %s alpn=%s resumed=%v", tls.VersionName(cs.Version), tls.CipherSuiteName(cs.CipherSuite), cs.CurveID, alpn, cs.DidResume)
}

// startMatrixPeers starts the Go servers the Xo client calls (name ->
// address): each certificate on TLS 1.3 and 1.2, TLS 1.3 with one group
// (a HelloRetryRequest for an X25519 share), TLS 1.2 with each suite and
// each group, ALPN h2 and http/1.1 only. Their handler answers with what
// the server saw.
func startMatrixPeers(t *testing.T, certs []matrixCert) map[string]string {
	t.Helper()
	peers := map[string]string{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s", r.Proto, matrixState(*r.TLS))
	})
	serve := func(name string, cfg *tls.Config) {
		cfg.NextProtos = append(cfg.NextProtos, "h2", "http/1.1")
		if strings.HasSuffix(name, "alpn http/1.1") {
			cfg.NextProtos = []string{"http/1.1"}
		}
		l, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: handler, ErrorLog: log.New(io.Discard, "", 0)}
		go srv.Serve(l)
		t.Cleanup(func() { srv.Close() })
		peers[name] = l.Addr().String()
	}
	for _, c := range certs {
		cert := []tls.Certificate{c.tls}
		serve("go "+c.name+" tls1.3", &tls.Config{Certificates: cert, MinVersion: tls.VersionTLS13})
		serve("go "+c.name+" tls1.2", &tls.Config{Certificates: cert, MaxVersion: tls.VersionTLS12})
		for _, s := range matrixSuites12(c.ecdsaSuite) {
			serve("go "+c.name+" tls1.2 "+tls.CipherSuiteName(s), &tls.Config{Certificates: cert, MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{s}})
		}
	}
	p := []tls.Certificate{certs[1].tls}
	for _, g := range matrixClientGroups {
		serve("go p256 tls1.3 "+g.name, &tls.Config{Certificates: p, MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{g.id}})
		serve("go p256 tls1.2 "+g.name, &tls.Config{Certificates: p, MaxVersion: tls.VersionTLS12, CurvePreferences: []tls.CurveID{g.id}})
	}
	for _, g := range matrixHybrids {
		serve("go p256 tls1.3 "+g.name, &tls.Config{Certificates: p, MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{g.id}})
	}
	serve("go p256 tls1.3 alpn http/1.1", &tls.Config{Certificates: p, MinVersion: tls.VersionTLS13})
	serve("go p256 tls1.2 alpn http/1.1", &tls.Config{Certificates: p, MaxVersion: tls.VersionTLS12})
	return peers
}

// matrixProgram is the Xo program: its client calls each Go peer, then
// starts one HTTPS server per certificate, calls them itself, prints
// their addresses, and serves until a stop request.
func matrixProgram(caPEM []byte, certs []matrixCert, peers map[string]string) string {
	var names []string
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(`use std/http

fn show(os: Os, label: Str, r: Result[http.Response, http.ClientErr]) {
  match r {
    Ok(resp) => os.stdio.println("${label}: ${resp.status} ${resp.text() ?? "?"}")
    Err(e) => os.stdio.println("${label}: ${e}")
  }
}

fn routes() -> http.Router {
  var r = http.router()
  r.handle("GET /x", fn(req) { http.text(200, "xo ${req.method} ${req.path}") })
  r
}

fn main(os: Os) {
`)
	fmt.Fprintf(&b, "  let c = http.client(os.net, timeout: 10s).with_roots(%q.bytes()).or_fault(\"roots\")\n", caPEM)
	for _, n := range names {
		fmt.Fprintf(&b, "  show(os, %q, c.get(\"https://%s/x\"))\n", "xo client -> "+n, peers[n])
	}
	var ls []string
	for i, mc := range certs {
		fmt.Fprintf(&b, "  let cert%d = http.tls_cert(%q.bytes(), %q.bytes()).or_fault(%q)\n", i, mc.cert, mc.key, mc.name)
		fmt.Fprintf(&b, "  let l%d = os.net.listen(\"127.0.0.1:0\").or_fault(\"listen\")\n", i)
		ls = append(ls, fmt.Sprintf("http.server(l%d, routes()).with_tls([cert%d])", i, i))
	}
	b.WriteString("  let servers = [" + strings.Join(ls, ", ") + "]\n")
	// a plain server in the foreground until a stop request; the TLS
	// servers in the background from the start
	b.WriteString("  let lp = os.net.listen(\"127.0.0.1:0\").or_fault(\"listen\")\n")
	b.WriteString("  scope s {\n    for srv in servers {\n      s.spawn_background(fn() {\n        match srv.serve() {\n          Ok(_) => ()\n          Err(e) => os.stdio.println(\"serve: ${e}\")\n        }\n      })\n    }\n")
	for i, mc := range certs {
		fmt.Fprintf(&b, "    show(os, %q, c.get(\"https://${l%d.addr()}/x\"))\n", "xo client -> xo "+mc.name, i)
	}
	for i, mc := range certs {
		fmt.Fprintf(&b, "    os.stdio.println(\"ADDR %s ${l%d.addr()}\")\n", mc.name, i)
	}
	b.WriteString("    os.stdio.println(\"READY\")\n    match http.server(lp, routes()).serve() {\n      Ok(_) => os.stdio.println(\"served\")\n      Err(e) => os.stdio.println(\"serve: ${e}\")\n    }\n  }\n}\n")
	return b.String()
}

// probeMatrix connects Go clients of each configuration to the Xo
// servers: TLS 1.3 with each group, a HelloRetryRequest (a share the
// server does not take), ALPN, resumption; TLS 1.2 with each suite and
// group, ALPN, resumption.
func probeMatrix(out *strings.Builder, addrs map[string]string, certs []matrixCert, roots *x509.CertPool) {
	dial := func(label, addr string, cfg *tls.Config, twice bool) {
		cfg.RootCAs = roots
		if twice {
			cfg.ClientSessionCache = tls.NewLRUClientSessionCache(4)
		}
		var got []string
		for range map[bool]int{false: 1, true: 2}[twice] {
			c, err := tls.Dial("tcp", addr, cfg)
			if err != nil {
				got = append(got, err.Error())
				break
			}
			// a request and its response: a TLS 1.3 ticket comes with them
			c.SetDeadline(time.Now().Add(10 * time.Second))
			fmt.Fprintf(c, "GET /x HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
			body, _ := io.ReadAll(c)
			status := "?"
			if i := bytes.IndexByte(body, '\r'); i > 0 {
				status = string(body[:i])
			}
			got = append(got, matrixState(c.ConnectionState())+" "+status)
			c.Close()
		}
		fmt.Fprintf(out, "go client -> xo %s: %s\n", label, strings.Join(got, " | "))
	}
	for _, mc := range certs {
		a := addrs[mc.name]
		dial(mc.name+" tls1.3", a, &tls.Config{MinVersion: tls.VersionTLS13}, false)
		dial(mc.name+" tls1.2", a, &tls.Config{MaxVersion: tls.VersionTLS12}, false)
		for _, s := range matrixSuites12(mc.ecdsaSuite) {
			dial(mc.name+" tls1.2 "+tls.CipherSuiteName(s), a, &tls.Config{MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{s}}, false)
		}
	}
	a := addrs["p256"]
	for _, g := range matrixGroups {
		dial("p256 tls1.3 "+g.name, a, &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{g.id}}, false)
		dial("p256 tls1.2 "+g.name, a, &tls.Config{MaxVersion: tls.VersionTLS12, CurvePreferences: []tls.CurveID{g.id}}, false)
	}
	for _, g := range matrixHybrids {
		dial("p256 tls1.3 "+g.name, a, &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{g.id}}, false)
		// with the X25519 hybrid too: Go's server order takes the first
		// hybrid (post-quantum first), the Xo server must agree
		dial("p256 tls1.3 "+g.name+" and default", a, &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{g.id, tls.X25519MLKEM768, tls.X25519}}, false)
	}
	for _, p := range [][]string{{"h2"}, {"http/1.1"}, {"h2", "http/1.1"}} {
		dial("p256 tls1.3 alpn "+strings.Join(p, ","), a, &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: p}, false)
		dial("p256 tls1.2 alpn "+strings.Join(p, ","), a, &tls.Config{MaxVersion: tls.VersionTLS12, NextProtos: p}, false)
	}
	for _, mc := range certs {
		dial(mc.name+" tls1.3 resume", addrs[mc.name], &tls.Config{MinVersion: tls.VersionTLS13}, true)
		dial(mc.name+" tls1.2 resume", addrs[mc.name], &tls.Config{MaxVersion: tls.VersionTLS12}, true)
	}
}

// TestTLSInteropMatrix is decision 0202's interop matrix: the Xo client
// against Go's servers, Go's clients against the Xo servers, and the Xo
// client against the Xo servers, across TLS 1.3 and 1.2, suites (GCM,
// ChaCha20-Poly1305, CBC on 1.2), groups (X25519, P-256, P-384, a
// HelloRetryRequest, P-521, and Go 1.26's SecP256r1MLKEM768 and
// SecP384r1MLKEM1024 on TLS 1.3), certificate keys (RSA, ECDSA P-256,
// P-384 and P-521, Ed25519), ALPN, and resumption. The program built with the Go backend
// is crypto/tls on both ends: the native backends must print the same.
// TLS 1.3's suite is the server's choice among Go's offers (AES-128-GCM);
// ChaCha20-Poly1305 and AES-256-GCM on TLS 1.3 are covered by the
// tls13_server and tls13_go goldens, a server's HelloRetryRequest to
// Go's client by tls13_server's recorded trace (Go's client offers
// X25519MLKEM768, which a Go server takes without one).
func TestTLSInteropMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a TLS program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	caPEM, roots, all := newMatrixPKI(t)
	peers := startMatrixPeers(t, all)
	var certs []matrixCert
	for _, c := range all {
		if !c.goOnly {
			certs = append(certs, c)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(matrixProgram(caPEM, certs, peers)), 0o644); err != nil {
		t.Fatal(err)
	}
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	outs := make([]string, len(backends))
	for i, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil {
			t.Fatalf("build (%s): %v", be, err)
		}
		if res.Failed {
			t.Fatalf("build (%s) failed: %v", be, res.Output)
		}
		outs[i] = runMatrix(t, res.Binaries[xotool.Host()], certs, roots)
	}
	t.Logf("go backend:\n%s", outs[0])
	// X25519MLKEM768, Go's default: the Xo server takes it (mlkem.xo)
	// and the Xo client offers it first (tls13.xo, B31 part 17)
	want := strings.Split(outs[0], "\n")
	for i, be := range backends[1:] {
		got := strings.Split(outs[i+1], "\n")
		if len(got) != len(want) {
			t.Errorf("%s: %d lines, go %d\n%s", be, len(got), len(want), outs[i+1])
			continue
		}
		n, known := 0, 0
		for j := range want {
			if got[j] == want[j] {
				continue
			}
			if strings.HasPrefix(want[j], "xo client -> go ") && strings.Contains(want[j], "_CBC_SHA:") &&
				!strings.Contains(want[j], ": 200 ") && !strings.Contains(got[j], ": 200 ") {
				// HTTP/2 over CBC: Go's server refuses it (GOAWAY
				// INADEQUATE_SECURITY), and Go's client words the end as
				// PROTOCOL_ERROR or a broken pipe, as the race goes; both fail
				continue
			}
			if why := matrixKnown(want[j]); why != "" {
				known++
				t.Logf("%s known difference (%s):\n  go:  %s\n  %s: %s", be, why, want[j], be, got[j])
				continue
			}
			n++
			t.Errorf("%s differs:\n  go:  %s\n  %s: %s", be, want[j], be, got[j])
		}
		t.Logf("%s: %d of %d cells differ, %d known", be, n, len(want), known)
	}
}

// matrixKnown is why a cell may differ for now (its owner's open work),
// or "".
func matrixKnown(cell string) string {
	_ = cell
	return ""
}

// runMatrix runs the program, probes its servers, stops it, and returns
// the matrix's lines (sorted).
func runMatrix(t *testing.T, bin string, certs []matrixCert, roots *x509.CertPool) string {
	t.Helper()
	cmd := exec.Command(bin)
	stopAttr(cmd)
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
	var lines []string
	addrs := map[string]string{}
	rd := bufio.NewReader(stdout)
	// a program that hangs is killed after 3 minutes: the lines so far
	// say where
	watchdog := time.AfterFunc(3*time.Minute, func() { cmd.Process.Kill() })
	defer watchdog.Stop()
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("program ended before serving: %v\n%s\nstderr:\n%s", err, strings.Join(lines, "\n"), stderr.String())
		}
		line = strings.TrimSpace(line)
		if line == "READY" {
			break
		}
		if a, ok := strings.CutPrefix(line, "ADDR "); ok {
			f := strings.SplitN(a, " ", 2)
			addrs[f[0]] = f[1]
			continue
		}
		lines = append(lines, line)
	}
	var out strings.Builder
	probeMatrix(&out, addrs, certs, roots)
	lines = append(lines, strings.Split(strings.TrimSpace(out.String()), "\n")...)
	if err := sendStop(cmd.Process); err != nil {
		t.Fatal(err)
	}
	io.ReadAll(rd)
	cmd.Wait()
	done = true
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
