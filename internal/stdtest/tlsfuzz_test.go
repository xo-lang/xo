package stdtest

// A differential of the std/tls server against Go's crypto/tls server on
// ClientHellos from a client it does not trust (task #90, std Proven
// evidence for tls: decision 0461 asks a harness of every parser of
// untrusted input).
//
// Go's tls.Client makes each hello with a seeded random config: versions
// 1.0 to 1.3 as the floor and the ceiling, a subset of every suite Go
// knows (insecure ones too), a subset of the curves (X25519MLKEM768
// too), ALPN protocols, a server name or none, session tickets on or
// off. A quarter of the hellos are changed after they are made, as a
// hostile client could: an extension dropped, repeated, moved or added,
// a byte of an extension body changed, legacy_version or the compression
// methods changed, no cipher suites, an extension body cut short with
// the outer lengths kept right; and now and then a byte of the record
// changed or the record cut.
//
// Each hello goes to both servers (the same ECDSA P-256 certificate,
// ALPN h2 and http/1.1, the default config otherwise) over its own TCP
// connection, then the end of stream. The test reads what each server
// writes back until it closes and compares a summary of it: for a
// ServerHello the version, cipher suite, key share group (or a
// HelloRetryRequest and its group), session id length, extension ids,
// ALPN, the plaintext handshake messages after it and the ECDHE curve
// of a 1.2 ServerKeyExchange; else the alert, a close without one, or a
// timeout.
//
// XO_TLSFUZZ_SEED (default 1), XO_TLSFUZZ_N (default 300) and
// XO_TLSFUZZ_BACKENDS (default arm64,llvm). The reference is Go's own
// crypto/tls: nothing to build.

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// tlfCapture is a net.Conn that keeps the first write (the client's
// first flight, its ClientHello) and fails the read after it.
type tlfCapture struct {
	net.Conn
	out []byte
}

func (c *tlfCapture) Write(b []byte) (int, error) {
	if c.out == nil {
		c.out = append([]byte(nil), b...)
	}
	return len(b), nil
}
func (c *tlfCapture) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *tlfCapture) Close() error                     { return nil }
func (c *tlfCapture) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *tlfCapture) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *tlfCapture) SetDeadline(time.Time) error      { return nil }
func (c *tlfCapture) SetReadDeadline(time.Time) error  { return nil }
func (c *tlfCapture) SetWriteDeadline(time.Time) error { return nil }

// tlfRand is a seeded io.Reader for Config.Rand.
type tlfRand struct{ r *rand.Rand }

func (r tlfRand) Read(b []byte) (int, error) { return r.r.Read(b) }

var tlfVersions = []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13}

var tlfCurves = []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521, tls.X25519MLKEM768}

var tlfProtos = []string{"h2", "http/1.1", "spdy/3", "x", "acme-tls/1"}

// tlfHello makes a ClientHello record with a random config.
func tlfHello(r *rand.Rand) ([]byte, string) {
	cfg := &tls.Config{InsecureSkipVerify: true, Rand: tlfRand{rand.New(rand.NewSource(r.Int63()))}}
	var desc []string
	lo, hi := r.Intn(4), r.Intn(4)
	if lo > hi {
		lo, hi = hi, lo
	}
	cfg.MinVersion, cfg.MaxVersion = tlfVersions[lo], tlfVersions[hi]
	desc = append(desc, fmt.Sprintf("v%x-%x", cfg.MinVersion, cfg.MaxVersion))
	if r.Intn(3) > 0 {
		var all []uint16
		for _, s := range tls.CipherSuites() {
			all = append(all, s.ID)
		}
		for _, s := range tls.InsecureCipherSuites() {
			all = append(all, s.ID)
		}
		r.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
		cfg.CipherSuites = all[:1+r.Intn(len(all))]
		desc = append(desc, fmt.Sprintf("suites%x", cfg.CipherSuites))
	}
	if r.Intn(3) > 0 {
		cs := append([]tls.CurveID(nil), tlfCurves...)
		r.Shuffle(len(cs), func(i, j int) { cs[i], cs[j] = cs[j], cs[i] })
		cfg.CurvePreferences = cs[:1+r.Intn(len(cs))]
		desc = append(desc, fmt.Sprintf("curves%v", cfg.CurvePreferences))
	}
	if r.Intn(2) == 0 {
		ps := append([]string(nil), tlfProtos...)
		r.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
		cfg.NextProtos = ps[:1+r.Intn(len(ps))]
		desc = append(desc, fmt.Sprintf("alpn%v", cfg.NextProtos))
	}
	switch r.Intn(3) {
	case 0:
		cfg.ServerName = "example.com"
	case 1:
		cfg.ServerName = "other.example"
	}
	cfg.SessionTicketsDisabled = r.Intn(3) == 0
	c := &tlfCapture{}
	tc := tls.Client(c, cfg)
	tc.Handshake() // fails at the read: the hello is written
	return c.out, strings.Join(desc, " ")
}

// tlfCH is a parsed ClientHello body.
type tlfCH struct {
	vers    []byte
	random  []byte
	sid     []byte
	suites  []byte
	comp    []byte
	exts    [][2][]byte // id (2 bytes), body
	hasExts bool
}

func tlfParse(rec []byte) (*tlfCH, bool) {
	if len(rec) < 9 || rec[0] != 22 || rec[5] != 1 {
		return nil, false
	}
	b := rec[9:]
	take := func(n int) []byte {
		if n > len(b) {
			n = len(b)
		}
		x := b[:n]
		b = b[n:]
		return x
	}
	vec := func(lenBytes int) []byte {
		l := 0
		for _, x := range take(lenBytes) {
			l = l<<8 | int(x)
		}
		return take(l)
	}
	h := &tlfCH{}
	h.vers = take(2)
	h.random = take(32)
	h.sid = vec(1)
	h.suites = vec(2)
	h.comp = vec(1)
	if len(b) == 0 {
		return h, true
	}
	h.hasExts = true
	e := vec(2)
	for len(e) >= 4 {
		id := e[:2]
		l := int(binary.BigEndian.Uint16(e[2:4]))
		if 4+l > len(e) {
			return nil, false
		}
		h.exts = append(h.exts, [2][]byte{id, e[4 : 4+l]})
		e = e[4+l:]
	}
	return h, true
}

func tlfEncode(h *tlfCH) []byte {
	var body []byte
	body = append(body, h.vers...)
	body = append(body, h.random...)
	body = append(body, byte(len(h.sid)))
	body = append(body, h.sid...)
	body = binary.BigEndian.AppendUint16(body, uint16(len(h.suites)))
	body = append(body, h.suites...)
	body = append(body, byte(len(h.comp)))
	body = append(body, h.comp...)
	if h.hasExts {
		var e []byte
		for _, x := range h.exts {
			e = append(e, x[0]...)
			e = binary.BigEndian.AppendUint16(e, uint16(len(x[1])))
			e = append(e, x[1]...)
		}
		body = binary.BigEndian.AppendUint16(body, uint16(len(e)))
		body = append(body, e...)
	}
	hs := []byte{1, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	hs = append(hs, body...)
	rec := []byte{22, 3, 1, byte(len(hs) >> 8), byte(len(hs))}
	return append(rec, hs...)
}

// tlfChange changes a hello as a hostile client could; it returns the
// change made.
func tlfChange(r *rand.Rand, rec []byte) ([]byte, string) {
	h, ok := tlfParse(rec)
	if !ok {
		return rec, ""
	}
	k := r.Intn(13)
	if len(h.exts) == 0 && k <= 4 {
		k = 5
	}
	cp := func(b []byte) []byte { return append([]byte(nil), b...) }
	switch k {
	case 0:
		i := r.Intn(len(h.exts))
		id := binary.BigEndian.Uint16(h.exts[i][0])
		h.exts = append(h.exts[:i:i], h.exts[i+1:]...)
		return tlfEncode(h), fmt.Sprintf("drop ext %d", id)
	case 1:
		i := r.Intn(len(h.exts))
		h.exts = append(h.exts, [2][]byte{cp(h.exts[i][0]), cp(h.exts[i][1])})
		return tlfEncode(h), fmt.Sprintf("repeat ext %d", binary.BigEndian.Uint16(h.exts[i][0]))
	case 2:
		r.Shuffle(len(h.exts), func(i, j int) { h.exts[i], h.exts[j] = h.exts[j], h.exts[i] })
		return tlfEncode(h), "shuffle exts"
	case 3:
		body := make([]byte, r.Intn(8))
		r.Read(body)
		at := r.Intn(len(h.exts) + 1)
		x := [2][]byte{{0xfa, 0xfa}, body}
		h.exts = append(h.exts[:at], append([][2][]byte{x}, h.exts[at:]...)...)
		return tlfEncode(h), "unknown ext"
	case 4:
		i := r.Intn(len(h.exts))
		id := binary.BigEndian.Uint16(h.exts[i][0])
		b := cp(h.exts[i][1])
		if len(b) == 0 {
			b = []byte{byte(r.Intn(256))}
			h.exts[i][1] = b
			return tlfEncode(h), fmt.Sprintf("ext %d body added", id)
		}
		if r.Intn(2) == 0 {
			b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
			h.exts[i][1] = b
			return tlfEncode(h), fmt.Sprintf("ext %d byte", id)
		}
		h.exts[i][1] = b[:r.Intn(len(b))]
		return tlfEncode(h), fmt.Sprintf("ext %d cut", id)
	case 5:
		v := []uint16{0x0300, 0x0302, 0x0304, 0x0305, 0x0200}[r.Intn(5)]
		h.vers = binary.BigEndian.AppendUint16(nil, v)
		return tlfEncode(h), fmt.Sprintf("legacy_version %x", v)
	case 6:
		h.suites = nil
		return tlfEncode(h), "no suites"
	case 7:
		h.comp = []byte{1}
		return tlfEncode(h), "no null compression"
	case 8:
		if len(h.suites) == 0 {
			h.suites = []byte{0x13}
		} else {
			h.suites = h.suites[:len(h.suites)-1]
		}
		return tlfEncode(h), "odd suites"
	case 9:
		h.sid = make([]byte, 33)
		return tlfEncode(h), "session id 33"
	case 10:
		h.hasExts, h.exts = false, nil
		return tlfEncode(h), "no extensions"
	case 11:
		b := cp(rec)
		at := 5 + r.Intn(len(b)-5)
		b[at] ^= byte(1 << r.Intn(8))
		return b, fmt.Sprintf("record byte %d", at)
	}
	n := r.Intn(len(rec))
	return cp(rec[:n]), fmt.Sprintf("record cut at %d", n)
}

// tlfDial dials the server under test (a variable so a test can make it
// fail).
var tlfDial = func(addr string) (net.Conn, error) { return net.DialTimeout("tcp", addr, 3*time.Second) }

// tlfDialRetry dials addr, waiting out a client with no free local port
// (EADDRNOTAVAIL: 10,000 cases a seed leave closed connections in
// TIME_WAIT, and a fuzz run beside it uses ports too; seed 3 lost 772 cases
// to "can't assign requested address" on the Mac, 2026-11-17).
func tlfDialRetry(addr string) (net.Conn, error) {
	deadline := time.Now().Add(2 * time.Minute)
	wait := 10 * time.Millisecond
	for {
		c, err := tlfDial(addr)
		if err == nil || !errors.Is(err, syscall.EADDRNOTAVAIL) || time.Now().After(deadline) {
			return c, err
		}
		time.Sleep(wait)
		if wait < time.Second {
			wait *= 2
		}
	}
}

// tlfSend sends a hello to addr, ends the stream and summarizes the reply.
func tlfSend(addr string, hello []byte) string {
	c, err := tlfDialRetry(addr)
	if err != nil {
		return "dial " + err.Error()
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write(hello); err != nil {
		return "write " + err.Error()
	}
	c.(*net.TCPConn).CloseWrite()
	var buf bytes.Buffer
	_, err = io.Copy(&buf, c)
	var ne net.Error
	timeout := err != nil && errors.As(err, &ne) && ne.Timeout()
	return tlfSummary(buf.Bytes(), timeout)
}

// tlfSummary reads the plaintext records a server wrote.
func tlfSummary(b []byte, timeout bool) string {
	var parts []string
	var hs []byte
	for len(b) >= 5 {
		typ, l := b[0], int(binary.BigEndian.Uint16(b[3:5]))
		if 5+l > len(b) {
			parts = append(parts, fmt.Sprintf("short record %d", typ))
			b = nil
			break
		}
		frag := b[5 : 5+l]
		b = b[5+l:]
		switch typ {
		case 22:
			hs = append(hs, frag...)
		case 21:
			if len(frag) == 2 {
				parts = append(parts, fmt.Sprintf("alert %d %d", frag[0], frag[1]))
			} else {
				parts = append(parts, fmt.Sprintf("alert len %d", len(frag)))
			}
		case 20:
			parts = append(parts, "ccs")
		case 23:
			if len(parts) == 0 || parts[len(parts)-1] != "encrypted" {
				parts = append(parts, "encrypted")
			}
		default:
			parts = append(parts, fmt.Sprintf("record %d", typ))
		}
	}
	if len(b) > 0 {
		parts = append(parts, fmt.Sprintf("trailing %d", len(b)))
	}
	var msgs []string
	for len(hs) >= 4 {
		mt, l := hs[0], int(hs[1])<<16|int(hs[2])<<8|int(hs[3])
		if 4+l > len(hs) {
			msgs = append(msgs, fmt.Sprintf("short %d", mt))
			break
		}
		body := hs[4 : 4+l]
		hs = hs[4+l:]
		switch mt {
		case 2:
			msgs = append(msgs, tlfServerHello(body))
		case 12:
			if len(body) >= 3 && body[0] == 3 {
				msgs = append(msgs, fmt.Sprintf("ske curve %d", binary.BigEndian.Uint16(body[1:3])))
			} else {
				msgs = append(msgs, "ske")
			}
		default:
			msgs = append(msgs, fmt.Sprintf("hs %d", mt))
		}
	}
	out := strings.Join(msgs, "; ")
	if len(parts) > 0 {
		if out != "" {
			out += " | "
		}
		out += strings.Join(parts, ", ")
	}
	if out == "" {
		out = "close"
	}
	if timeout {
		out += " (timeout)"
	}
	return out
}

var tlfHRR = []byte{0xCF, 0x21, 0xAD, 0x74, 0xE5, 0x9A, 0x61, 0x11, 0xBE, 0x1D, 0x8C, 0x02, 0x1E, 0x65, 0xB8, 0x91, 0xC2, 0xA2, 0x11, 0x16, 0x7A, 0xBB, 0x8C, 0x5E, 0x07, 0x9E, 0x09, 0xE2, 0xC8, 0xA8, 0x33, 0x9C}

func tlfServerHello(b []byte) string {
	if len(b) < 38 {
		return fmt.Sprintf("serverhello len %d", len(b))
	}
	vers := binary.BigEndian.Uint16(b[:2])
	kind := "serverhello"
	if bytes.Equal(b[2:34], tlfHRR) {
		kind = "hrr"
	}
	sl := int(b[34])
	if 35+sl+3 > len(b) {
		return kind + " short"
	}
	rest := b[35+sl:]
	suite := binary.BigEndian.Uint16(rest[:2])
	comp := rest[2]
	rest = rest[3:]
	var ids []int
	alpn, group := "", ""
	if len(rest) >= 2 {
		e := rest[2:]
		for len(e) >= 4 {
			id := int(binary.BigEndian.Uint16(e[:2]))
			l := int(binary.BigEndian.Uint16(e[2:4]))
			if 4+l > len(e) {
				break
			}
			body := e[4 : 4+l]
			e = e[4+l:]
			ids = append(ids, id)
			switch id {
			case 43:
				if len(body) == 2 {
					vers = binary.BigEndian.Uint16(body)
				}
			case 51:
				if len(body) >= 2 {
					group = fmt.Sprintf(" group %d", binary.BigEndian.Uint16(body[:2]))
				}
			case 16:
				if len(body) > 3 {
					alpn = " alpn " + string(body[3:])
				}
			}
		}
	}
	sort.Ints(ids)
	return fmt.Sprintf("%s v%x suite %x comp %d sid %d%s%s exts %v", kind, vers, suite, comp, sl, group, alpn, ids)
}

// tlfCert writes a self-signed ECDSA P-256 certificate for 127.0.0.1 and
// example.com and its PKCS #8 key into dir.
func tlfCert(t *testing.T, dir string) (certFile, keyFile string, cert tls.Certificate) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"example.com"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.New(rand.NewSource(8)), tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cp := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	kp := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder})
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, cp, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, kp, 0o600); err != nil {
		t.Fatal(err)
	}
	cert, err = tls.X509KeyPair(cp, kp)
	if err != nil {
		t.Fatal(err)
	}
	return
}

// tlfGoServer serves Go's crypto/tls server on loopback.
func tlfGoServer(t *testing.T, cert tls.Certificate) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				tc := tls.Server(c, cfg)
				tc.Handshake()
				tc.Close()
			}()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// tlfRun sends every hello to addr, 8 at a time.
func tlfRun(addr string, hellos [][]byte) []string {
	out := make([]string, len(hellos))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i] = tlfSend(addr, hellos[i])
			}
		}()
	}
	for i := range hellos {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

func TestTlsFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the tls fuzz builds the Xo server")
	}
	seed := int64(envInt(t, "XO_TLSFUZZ_SEED", 1))
	n := envInt(t, "XO_TLSFUZZ_N", 300)
	backends := envBackends("XO_TLSFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	hellos := make([][]byte, n)
	notes := make([]string, n)
	for i := range hellos {
		h, desc := tlfHello(r)
		note := ""
		if r.Intn(4) == 0 {
			h, note = tlfChange(r, h)
		}
		hellos[i], notes[i] = h, desc+" / "+note
	}
	work, err := testtmp.Dir("xo-tlsfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	certFile, keyFile, cert := tlfCert(t, work)
	goAddr, stop := tlfGoServer(t, cert)
	defer stop()
	want := tlfRun(goAddr, hellos)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range backends {
		bin := filepath.Join(work, "xo-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "tlsdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		cmd := testtmp.Guard(exec.Command(bin, certFile, keyFile))
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var errb bytes.Buffer
		cmd.Stderr = &errb
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(stdout).ReadString('\n')
		addr, ok := strings.CutPrefix(strings.TrimSpace(line), "addr ")
		if err != nil || !ok {
			stdin.Close()
			cmd.Wait()
			t.Fatalf("%s: no address: %q %v\nstderr:\n%s", b, line, err, myfTail(errb.String(), 30))
		}
		got := tlfRun(addr, hellos)
		stdin.Close()
		werr := cmd.Wait()
		bad, strict := 0, 0
		for i := range hellos {
			if got[i] == want[i] {
				continue
			}
			if tlfLongSID(notes[i], got[i]) {
				strict++
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d, %s):\ngot  %s\nwant %s\nhello %x", b, i, seed, notes[i], got[i], want[i], hellos[i])
			}
			bad++
		}
		if werr != nil {
			t.Errorf("%s: the server exited: %v\nstderr:\n%s", b, werr, myfTail(errb.String(), 30))
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d hellos answered as Go's crypto/tls server answers them, %d with a session id over 32 bytes refused (seed %d)", b, n-strict, strict, seed)
	}
}

// tlfLongSID: the one difference the test accepts. Go reads a
// legacy_session_id of any length and echoes it; Xo keeps RFC 8446's
// limit (4.1.2, at most 32 bytes) and answers with a decode_error alert
// and no ServerHello (std/evidence/tls/fuzz.md).
func tlfLongSID(note, got string) bool {
	return strings.Contains(note, "session id 33") && !strings.Contains(got, "serverhello") && strings.Contains(got, "2 50")
}
