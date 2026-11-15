package rt

// std/tls on the Go backend (spec/std.md 27, decision 0281): crypto/tls's
// client on a connection the program dialed. std/native/tls.xo is the
// native backends'.

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// upgradedConns are the connections std/tls took over: their own read
// and write fail (a Conn is copied by value, so the mark lives here).
var upgradedConns sync.Map

const upgradedText = "use of a connection upgraded to TLS"

func connUpgraded(c net.Conn) bool {
	if c == nil {
		return false
	}
	_, ok := upgradedConns.Load(c)
	return ok
}

// Tls_Roots is a std/tls Roots: a certificate pool.
type Tls_Roots struct{ pool *x509.CertPool }

// XoDebug implements Debug.
func (Tls_Roots) XoDebug(b *strings.Builder) { b.WriteString("<roots>") }

// Tls_TlsConn is a std/tls TlsConn: the TLS client, or the plain
// connection on FakeNet (fake: the bytes pass through).
type Tls_TlsConn struct {
	t      *tls.Conn
	c      net.Conn
	fake   bool
	closed *atomic.Bool
}

// XoDebug implements Debug.
func (Tls_TlsConn) XoDebug(b *strings.Builder) { b.WriteString("<tlsconn>") }

// Tls_roots is `tls.roots(pem)`: the CERTIFICATE blocks of pem that parse.
func Tls_roots(_ *Task, pem Bytes) Result[Tls_Roots, Tls_TlsErr] {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(pem)) {
		return Err[Tls_Roots](Tls_TlsErr{F_msg: "tls: no certificates found in PEM input"})
	}
	return Ok[Tls_Roots, Tls_TlsErr](Tls_Roots{pool: pool})
}

// fakeConn marks a connection of the test world's FakeNet: tls.client
// passes its bytes through.
type fakeConn interface{ xoFakeNet() }

// Tls_TlsCert is a std/tls TlsCert, std/http's TlsCert too (decision
// 0433): a certificate chain with its private key.
type Tls_TlsCert struct{ c *tls.Certificate }

// XoDebug implements Debug.
func (Tls_TlsCert) XoDebug(b *strings.Builder) { b.WriteString("<tls cert>") }

// Tls_cert is `tls.cert(cert_pem, key_pem)`: tls.X509KeyPair.
func Tls_cert(_ *Task, certPEM, keyPEM Bytes) Result[Tls_TlsCert, Tls_TlsErr] {
	c, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return Err[Tls_TlsCert](Tls_TlsErr{F_msg: err.Error()})
	}
	return Ok[Tls_TlsCert, Tls_TlsErr](Tls_TlsCert{c: &c})
}

// clientAuthType is a std/tls ClientAuth as Go's (the same order).
func clientAuthType(m Tls_ClientAuth) tls.ClientAuthType { return tls.ClientAuthType(m.Tag - 1) }

// Tls_client is `tls.client(c, host, roots, verify, alpn, cert)`: the
// handshake on c (at most 10 seconds), which std/tls owns from then on.
func Tls_client(tk *Task, c Conn, host string, roots Option[Tls_Roots], verify Tls_Verify, alpn List[string], cert Option[Tls_TlsCert], verifyConn Option[func(*Task, List[X509_Certificate]) Result[Unit, string]], minVersion int64, maxVersion int64, cipherSuites List[int64], curvePrefs List[int64]) Result[Tls_TlsConn, NetErr] {
	return tlsClient(tk, c, host, roots, verify, alpn, cert, verifyConn, minVersion, maxVersion, cipherSuites, curvePrefs, nil)
}

// tlsClient is Tls_client with extra, when not nil, changing the
// configuration last (ClientConfig's hooks, decision 0644 T6).
func tlsClient(tk *Task, c Conn, host string, roots Option[Tls_Roots], verify Tls_Verify, alpn List[string], cert Option[Tls_TlsCert], verifyConn Option[func(*Task, List[X509_Certificate]) Result[Unit, string]], minVersion int64, maxVersion int64, cipherSuites List[int64], curvePrefs List[int64], extra func(*tls.Config)) Result[Tls_TlsConn, NetErr] {
	if tk.Canceled() {
		return Err[Tls_TlsConn](netErr(5, ""))
	}
	if c.c == nil {
		return Err[Tls_TlsConn](netErr(4, netClosedText))
	}
	if _, loaded := upgradedConns.LoadOrStore(c.c, struct{}{}); loaded {
		return Err[Tls_TlsConn](netErr(4, upgradedText))
	}
	if _, ok := c.c.(fakeConn); ok {
		return Ok[Tls_TlsConn, NetErr](Tls_TlsConn{c: c.c, fake: true, closed: new(atomic.Bool)})
	}
	name := host
	if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
		name = name[1 : len(name)-1]
	}
	cfg := &tls.Config{ServerName: name, NextProtos: alpn.s}
	setPrefs(cfg, minVersion, maxVersion, cipherSuites, curvePrefs)
	if roots.Some {
		cfg.RootCAs = roots.V.pool
	}
	if cert.Some {
		cfg.Certificates = []tls.Certificate{*cert.V.c}
	}
	switch verify.Tag {
	case 2:
		// ChainOnly: the chain against the roots, not the name (libpq's
		// verify-ca)
		cfg.InsecureSkipVerify = true
		pool := cfg.RootCAs
		cfg.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
			return verifyChain(raw, pool)
		}
	case 3:
		cfg.InsecureSkipVerify = true
	}
	if verifyConn.Some {
		// Go's VerifyConnection (decision 0541): after the verification mode's own check
		f := verifyConn.V
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			var certs []X509_Certificate
			for _, pc := range cs.PeerCertificates {
				certs = append(certs, x509CertOf(tk, pc))
			}
			if r := f(tk, ListFromSlice(certs)); !r.Ok {
				if r.E == "" {
					return errors.New("tls: verify_conn failed")
				}
				return errors.New(r.E)
			}
			return nil
		}
	}
	if extra != nil {
		extra(cfg)
	}
	t := tls.Client(c.c, cfg)
	// http.DefaultTransport's TLSHandshakeTimeout, or the connection's own
	// deadlines when earlier (decision 0430: Go's Handshake honors them)
	limit := time.Now().Add(10 * time.Second)
	for _, w := range []bool{false, true} {
		if d := userDeadline(c.c, w); !d.IsZero() && d.Before(limit) {
			limit = d
		}
	}
	c.c.SetDeadline(limit)
	canceled, err := interruptible(tk, c.c.SetDeadline, t.Handshake)
	c.c.SetReadDeadline(userDeadline(c.c, false))
	c.c.SetWriteDeadline(userDeadline(c.c, true))
	if canceled {
		return Err[Tls_TlsConn](netErr(5, ""))
	}
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return Err[Tls_TlsConn](netErr(3, ""))
		}
		return Err[Tls_TlsConn](netErr(4, err.Error()))
	}
	return Ok[Tls_TlsConn, NetErr](Tls_TlsConn{t: t, c: c.c, closed: new(atomic.Bool)})
}

// verifyChain is crypto/tls's verification without the host name: the
// leaf's chain to pool (nil: the system's roots) through the others.
func verifyChain(raw [][]byte, pool *x509.CertPool) error {
	certs := make([]*x509.Certificate, len(raw))
	for i, b := range raw {
		c, err := x509.ParseCertificate(b)
		if err != nil {
			return errors.New("tls: failed to parse certificate from server: " + err.Error())
		}
		certs[i] = c
	}
	if len(certs) == 0 {
		return errors.New("tls: received empty certificates message")
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	_, err := certs[0].Verify(x509.VerifyOptions{Roots: pool, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil {
		return &tls.CertificateVerificationError{UnverifiedCertificates: certs, Err: err}
	}
	return nil
}

// chainsOf is the chains verifyChain finds (Verify.ChainOnly's
// verified_chains for verify_peer_certificate; decision 0644 T6).
func chainsOf(raw [][]byte, pool *x509.CertPool) [][]*x509.Certificate {
	var certs []*x509.Certificate
	for _, b := range raw {
		c, err := x509.ParseCertificate(b)
		if err != nil {
			return nil
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	chains, _ := certs[0].Verify(x509.VerifyOptions{Roots: pool, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	return chains
}

// hookErr is a hook's Err as Go's error, def for an empty text.
func hookErr(e, def string) error {
	if e == "" {
		return errors.New(def)
	}
	return errors.New(e)
}

// tlsVerifyHooks sets cfg's VerifyPeerCertificate (after the one the
// verification mode set) and VerifyConnection from Xo's hooks.
func tlsVerifyHooks(tk *Task, cfg *tls.Config, vp Option[func(*Task, List[Bytes], List[List[X509_Certificate]]) Result[Unit, string]], vc Option[func(*Task, Tls_ConnState) Result[Unit, string]], chainOnly bool) {
	if vp.Some {
		prev := cfg.VerifyPeerCertificate
		pool := cfg.RootCAs
		if cfg.ClientCAs != nil {
			pool = cfg.ClientCAs
		}
		f := vp.V
		cfg.VerifyPeerCertificate = func(raw [][]byte, chains [][]*x509.Certificate) error {
			if prev != nil {
				if err := prev(raw, chains); err != nil {
					return err
				}
				if chainOnly {
					chains = chainsOf(raw, pool)
				}
			}
			var rs []Bytes
			for _, b := range raw {
				rs = append(rs, Bytes(b))
			}
			var cs []List[X509_Certificate]
			for _, ch := range chains {
				var l []X509_Certificate
				for _, x := range ch {
					l = append(l, x509CertOf(tk, x))
				}
				cs = append(cs, ListFromSlice(l))
			}
			if r := f(tk, ListFromSlice(rs), ListFromSlice(cs)); !r.Ok {
				return hookErr(r.E, "tls: verify_peer_certificate failed")
			}
			return nil
		}
	}
	if vc.Some {
		prev := cfg.VerifyConnection
		f := vc.V
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if prev != nil {
				if err := prev(cs); err != nil {
					return err
				}
			}
			if r := f(tk, connStateOf(tk, cs)); !r.Ok {
				return hookErr(r.E, "tls: verify_connection failed")
			}
			return nil
		}
	}
}

// Tls_client_config is `tls.client_config(c, config)` (decision 0644 T6).
func Tls_client_config(tk *Task, c Conn, cfg Tls_ClientConfig) Result[Tls_TlsConn, NetErr] {
	var one Option[Tls_TlsCert]
	if len(cfg.F_certificates.s) == 1 {
		one = Option[Tls_TlsCert]{Some: true, V: cfg.F_certificates.s[0]}
	}
	return tlsClient(tk, c, cfg.F_server_name, cfg.F_roots, cfg.F_verify, cfg.F_next_protos, one, Option[func(*Task, List[X509_Certificate]) Result[Unit, string]]{}, cfg.F_min_version, cfg.F_max_version, cfg.F_cipher_suites, cfg.F_curve_preferences, func(gc *tls.Config) {
		if len(cfg.F_certificates.s) > 1 {
			gc.Certificates = nil
			for _, x := range cfg.F_certificates.s {
				gc.Certificates = append(gc.Certificates, *x.c)
			}
		}
		tlsVerifyHooks(tk, gc, cfg.F_verify_peer_certificate, cfg.F_verify_connection, cfg.F_verify.Tag == 2)
		setKeyLog(tk, gc, cfg.F_key_log)
		if cfg.F_get_client_certificate.Some {
			g := cfg.F_get_client_certificate.V
			gc.GetClientCertificate = func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
				r := g(tk, criOf(cri))
				if !r.Ok {
					return nil, hookErr(r.E, "tls: get_client_certificate failed")
				}
				if !r.V.Some {
					return new(tls.Certificate), nil
				}
				return r.V.V.c, nil
			}
		}
	})
}

// criOf is Go's CertificateRequestInfo as Xo's.
func criOf(cri *tls.CertificateRequestInfo) Tls_CertificateRequestInfo {
	var cas []Bytes
	for _, b := range cri.AcceptableCAs {
		cas = append(cas, Bytes(b))
	}
	var ss []int64
	for _, x := range cri.SignatureSchemes {
		ss = append(ss, int64(x))
	}
	return Tls_CertificateRequestInfo{F_acceptable_cas: ListFromSlice(cas), F_signature_schemes: ListFromSlice(ss), F_version: int64(cri.Version)}
}

// M_supports_certificate is `i.supports_certificate(cert)`: Go's
// CertificateRequestInfo.SupportsCertificate.
func (v Tls_CertificateRequestInfo) M_supports_certificate(tk *Task, cert Tls_TlsCert) Result[Unit, Tls_TlsErr] {
	cri := &tls.CertificateRequestInfo{Version: uint16(v.F_version)}
	for _, b := range v.F_acceptable_cas.s {
		cri.AcceptableCAs = append(cri.AcceptableCAs, []byte(b))
	}
	for _, x := range v.F_signature_schemes.s {
		cri.SignatureSchemes = append(cri.SignatureSchemes, tls.SignatureScheme(x))
	}
	if err := cri.SupportsCertificate(cert.c); err != nil {
		return Err[Unit](Tls_TlsErr{F_msg: err.Error()})
	}
	return Ok[Unit, Tls_TlsErr](Unit{})
}

// Tls_dial_config is `tls.dial_config(net, addr, config, timeout)`.
func Tls_dial_config(tk *Task, n Net, addr string, cfg Tls_ClientConfig, timeout Option[Duration]) Result[Tls_TlsConn, NetErr] {
	if cfg.F_server_name == "" {
		cfg.F_server_name = addr
		if i := strings.LastIndex(addr, ":"); i >= 0 {
			cfg.F_server_name = addr[:i]
		}
	}
	if !timeout.Some {
		c := n.M_dial(tk, addr)
		if !c.Ok {
			return Err[Tls_TlsConn](c.E)
		}
		return Tls_client_config(tk, c.V, cfg)
	}
	start := time.Now()
	c := n.M_dial_timeout(tk, addr, timeout.V)
	if !c.Ok {
		return Err[Tls_TlsConn](c.E)
	}
	at := start.Add(time.Duration(int64(timeout.V)))
	if !time.Now().Before(at) {
		c.V.M_close(tk)
		return Err[Tls_TlsConn](netErr(3, ""))
	}
	setUserDeadline(c.V, at)
	r := Tls_client_config(tk, c.V, cfg)
	setUserDeadline(c.V, time.Time{})
	if !r.Ok {
		c.V.M_close(tk)
	}
	return r
}

// M_read is `t.read(max)`: at most max bytes (at most 1 MiB), None at the
// end of the stream.
func (t Tls_TlsConn) M_read(tk *Task, max int64) Result[Option[Bytes], NetErr] {
	if tk.Canceled() {
		return Err[Option[Bytes]](netErr(5, ""))
	}
	if t.fake {
		return passthrough(func() Result[Option[Bytes], NetErr] { return Conn{c: t.c}.M_read(tk, max) }, t.c)
	}
	if t.closed.Load() {
		return Err[Option[Bytes]](netErr(4, netClosedText))
	}
	if max <= 0 {
		return Ok[Option[Bytes], NetErr](Some(Bytes("")))
	}
	buf := make([]byte, min(max, 1<<20))
	var n int
	canceled, err := interruptibleTo(tk, t.c.SetReadDeadline, userDeadline(t.c, false), func() error {
		var e error
		n, e = t.t.Read(buf)
		return e
	})
	if canceled {
		return Err[Option[Bytes]](netErr(5, ""))
	}
	if n > 0 {
		return Ok[Option[Bytes], NetErr](Some(Bytes(buf[:n])))
	}
	if err == nil || errors.Is(err, io.EOF) {
		return Ok[Option[Bytes], NetErr](Option[Bytes]{})
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return Err[Option[Bytes]](netErr(3, ""))
	}
	return Err[Option[Bytes]](netIoErr(err))
}

// M_write is `t.write(data)`: writes every byte.
func (t Tls_TlsConn) M_write(tk *Task, data Bytes) Result[Unit, NetErr] {
	if tk.Canceled() {
		return Err[Unit](netErr(5, ""))
	}
	if t.fake {
		return passthrough(func() Result[Unit, NetErr] { return Conn{c: t.c}.M_write(tk, data) }, t.c)
	}
	if t.closed.Load() {
		return Err[Unit](netErr(4, netClosedText))
	}
	canceled, err := interruptibleTo(tk, t.c.SetWriteDeadline, userDeadline(t.c, true), func() error {
		_, e := t.t.Write([]byte(data))
		return e
	})
	if canceled {
		return Err[Unit](netErr(5, ""))
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return Err[Unit](netErr(3, ""))
	}
	if err != nil {
		return Err[Unit](netIoErr(err))
	}
	return Ok[Unit, NetErr](Unit{})
}

// passthrough runs op, a read or write of FakeNet's connection itself,
// with the connection's upgraded mark lifted.
func passthrough[R any](op func() R, c net.Conn) R {
	upgradedConns.Delete(c)
	defer upgradedConns.Store(c, struct{}{})
	return op()
}

// M_set_deadline is Go's tls.Conn SetDeadline (decision 0430): the
// connection's.
func (t Tls_TlsConn) M_set_deadline(tk *Task, clock Clock, at Option[Time]) {
	Conn{c: t.c}.setDeadline(tk, clock, at, true, true)
}

// M_set_read_deadline is Go's tls.Conn SetReadDeadline (decision 0430).
func (t Tls_TlsConn) M_set_read_deadline(tk *Task, clock Clock, at Option[Time]) {
	Conn{c: t.c}.setDeadline(tk, clock, at, true, false)
}

// M_set_write_deadline is Go's tls.Conn SetWriteDeadline (decision 0430).
func (t Tls_TlsConn) M_set_write_deadline(tk *Task, clock Clock, at Option[Time]) {
	Conn{c: t.c}.setDeadline(tk, clock, at, false, true)
}

// M_close is `t.close()`: close_notify, then the connection closes.
func (t Tls_TlsConn) M_close(*Task) {
	if t.closed.Swap(true) {
		return
	}
	if t.t != nil {
		t.t.Close()
		return
	}
	if t.c != nil {
		t.c.Close()
	}
}

// M_alpn is `t.alpn()`: the protocol the server chose, "" for none.
func (t Tls_TlsConn) M_alpn(*Task) string {
	if t.t == nil {
		return ""
	}
	return t.t.ConnectionState().NegotiatedProtocol
}

// M_version is `t.version()`: "TLS 1.2" or "TLS 1.3".
func (t Tls_TlsConn) M_version(*Task) string {
	if t.t == nil {
		return "TLS 1.3"
	}
	return tls.VersionName(t.t.ConnectionState().Version)
}

// pinCheck is the verify_conn function of pin_sha1 and pin_sha256 (decision 0541): the digest of the leaf's DER
// against the pin in constant time.
func pinCheck(pin string, size int, sum func([]byte) []byte) Result[func(*Task, List[X509_Certificate]) Result[Unit, string], string] {
	type fn = func(*Task, List[X509_Certificate]) Result[Unit, string]
	hex := strings.NewReplacer(":", "", " ", "").Replace(pin)
	want, err := hexDecode(hex)
	if err != nil {
		return Err[fn]("tls: pin is not hexadecimal")
	}
	if len(want) != size {
		return Err[fn](fmt.Sprintf("tls: pin is %d bytes, want %d", len(want), size))
	}
	return Ok[fn, string](func(_ *Task, certs List[X509_Certificate]) Result[Unit, string] {
		if len(certs.s) == 0 {
			return Err[Unit]("tls: server presented no certificate")
		}
		if subtle.ConstantTimeCompare(sum([]byte(certs.s[0].F_raw)), want) != 1 {
			return Err[Unit]("tls: certificate does not match the pin")
		}
		return Ok[Unit, string](Unit{})
	})
}

// Tls_pin_sha1 is `tls.pin_sha1(pin)`.
func Tls_pin_sha1(_ *Task, pin string) Result[func(*Task, List[X509_Certificate]) Result[Unit, string], string] {
	return pinCheck(pin, sha1.Size, func(b []byte) []byte { h := sha1.Sum(b); return h[:] })
}

// Tls_pin_sha256 is `tls.pin_sha256(pin)`.
func Tls_pin_sha256(_ *Task, pin string) Result[func(*Task, List[X509_Certificate]) Result[Unit, string], string] {
	return pinCheck(pin, sha256.Size, func(b []byte) []byte { h := sha256.Sum256(b); return h[:] })
}

func hexDecode(s string) ([]byte, error) { return hex.DecodeString(s) }

// Tls_dial is `tls.dial(net, addr, server_name, ...)`: Go's
// DialWithDialer, the timeout over the dial and the handshake together
// and cleared after.
func Tls_dial(tk *Task, n Net, addr string, serverName string, roots Option[Tls_Roots], verify Tls_Verify, alpn List[string], cert Option[Tls_TlsCert], verifyConn Option[func(*Task, List[X509_Certificate]) Result[Unit, string]], timeout Option[Duration], minVersion int64, maxVersion int64, cipherSuites List[int64], curvePrefs List[int64]) Result[Tls_TlsConn, NetErr] {
	name := serverName
	if name == "" {
		name = addr
		if i := strings.LastIndex(addr, ":"); i >= 0 {
			name = addr[:i]
		}
	}
	if !timeout.Some {
		c := n.M_dial(tk, addr)
		if !c.Ok {
			return Err[Tls_TlsConn](c.E)
		}
		return Tls_client(tk, c.V, name, roots, verify, alpn, cert, verifyConn, minVersion, maxVersion, cipherSuites, curvePrefs)
	}
	start := time.Now()
	c := n.M_dial_timeout(tk, addr, timeout.V)
	if !c.Ok {
		return Err[Tls_TlsConn](c.E)
	}
	at := start.Add(time.Duration(int64(timeout.V)))
	if !time.Now().Before(at) {
		c.V.M_close(tk)
		return Err[Tls_TlsConn](netErr(3, ""))
	}
	setUserDeadline(c.V, at)
	r := Tls_client(tk, c.V, name, roots, verify, alpn, cert, verifyConn, minVersion, maxVersion, cipherSuites, curvePrefs)
	setUserDeadline(c.V, time.Time{})
	if !r.Ok {
		c.V.M_close(tk)
	}
	return r
}

// setUserDeadline sets both of c's deadlines to at (zero: none), as
// set_deadline does, for the handshake tls.dial bounds.
func setUserDeadline(c Conn, at time.Time) {
	if c.c == nil {
		return
	}
	if _, ok := c.c.(*memEnd); ok {
		return
	}
	v, _ := connDeadlines.LoadOrStore(c.c, &connDL{})
	d := v.(*connDL)
	n := int64(0)
	if !at.IsZero() {
		n = at.UnixNano()
	}
	d.rd.Store(n)
	d.wr.Store(n)
	c.c.SetDeadline(at)
}

// Tls_load_cert is `tls.load_cert(fs, cert_file, key_file)`: Go's
// LoadX509KeyPair through fs.
func Tls_load_cert(tk *Task, fs Fs, certFile, keyFile string) Result[Tls_TlsCert, Tls_TlsErr] {
	cp := fs.M_read(tk, certFile)
	if !cp.Ok {
		return Err[Tls_TlsCert](Tls_TlsErr{F_msg: cp.E.M_display(tk)})
	}
	kp := fs.M_read(tk, keyFile)
	if !kp.Ok {
		return Err[Tls_TlsCert](Tls_TlsErr{F_msg: kp.E.M_display(tk)})
	}
	return Tls_cert(tk, cp.V, kp.V)
}

// M_net_conn is `t.net_conn()`: Go's Conn.NetConn.
func (t Tls_TlsConn) M_net_conn(*Task) Conn { return Conn{c: t.c} }

// M_verify_hostname is `t.verify_hostname(host)`: Go's Conn.VerifyHostname.
func (t Tls_TlsConn) M_verify_hostname(tk *Task, host string) Result[Unit, Tls_TlsErr] {
	if t.t == nil {
		return Err[Unit](Tls_TlsErr{F_msg: "tls: handshake did not verify certificate chain"})
	}
	if err := t.t.VerifyHostname(host); err != nil {
		return Err[Unit](Tls_TlsErr{F_msg: err.Error()})
	}
	return Ok[Unit, Tls_TlsErr](Unit{})
}

// M_local_addr is `t.local_addr()`: Go's Conn.LocalAddr.
func (t Tls_TlsConn) M_local_addr(tk *Task) string { return Conn{c: t.c}.M_local_addr(tk) }

// M_remote_addr is `t.remote_addr()`: Go's Conn.RemoteAddr.
func (t Tls_TlsConn) M_remote_addr(tk *Task) string { return Conn{c: t.c}.M_remote_addr(tk) }

// M_close_write is `t.close_write()`: Go's Conn.CloseWrite.
func (t Tls_TlsConn) M_close_write(tk *Task) Result[Unit, NetErr] {
	if t.closed.Load() {
		return Err[Unit](netErr(4, netClosedText))
	}
	if t.t == nil {
		return Conn{c: t.c}.M_close_write(tk)
	}
	if err := t.t.CloseWrite(); err != nil {
		return Err[Unit](netErr(4, err.Error()))
	}
	return Ok[Unit, NetErr](Unit{})
}

// Tls_TlsServerConn is a std/tls TlsServerConn (decision 0644 T1):
// crypto/tls's server Conn, whose handshake runs at the first read, write
// or handshake (tls.Server's); the methods it shares with TlsConn are
// TlsConn's.
type Tls_TlsServerConn struct {
	Tls_TlsConn
	noCerts bool
}

// Tls_TlsListener is a std/tls TlsListener: the listener and its config.
type Tls_TlsListener struct {
	l   Listener
	cfg Tls_ServerConfig
}

// XoDebug implements Debug.
func (Tls_TlsListener) XoDebug(b *strings.Builder) { b.WriteString("<tlslistener>") }

// serverConfig is cfg as crypto/tls's Config.
// keyLogWriter is Go's KeyLogWriter over std/tls's key_log (decision
// 0644 T8): crypto/tls writes each NSS line in one Write.
type keyLogWriter struct {
	tk *Task
	f  func(*Task, string) Result[Unit, string]
}

func (w keyLogWriter) Write(p []byte) (int, error) {
	if r := w.f(w.tk, string(p)); !r.Ok {
		return 0, hookErr(r.E, "tls: key_log failed")
	}
	return len(p), nil
}

func setKeyLog(tk *Task, c *tls.Config, kl Option[func(*Task, string) Result[Unit, string]]) {
	if kl.Some {
		c.KeyLogWriter = keyLogWriter{tk, kl.V}
	}
}

func serverConfig(tk *Task, cfg Tls_ServerConfig) *tls.Config {
	c := &tls.Config{NextProtos: cfg.F_next_protos.s, ClientAuth: clientAuthType(cfg.F_client_auth)}
	setPrefs(c, cfg.F_min_version, cfg.F_max_version, cfg.F_cipher_suites, cfg.F_curve_preferences)
	for _, tc := range cfg.F_certificates.s {
		c.Certificates = append(c.Certificates, *tc.c)
	}
	if cfg.F_client_cas.Some {
		c.ClientCAs = cfg.F_client_cas.V.pool
	}
	tlsVerifyHooks(tk, c, cfg.F_verify_peer_certificate, cfg.F_verify_connection, false)
	setKeyLog(tk, c, cfg.F_key_log)
	// decision 0644 T5: GetCertificate and GetConfigForClient
	if cfg.F_get_certificate.Some {
		g := cfg.F_get_certificate.V
		c.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			r := g(tk, chiOf(chi))
			if !r.Ok {
				return nil, hookErr(r.E, "tls: get_certificate failed")
			}
			if !r.V.Some {
				return nil, nil
			}
			return r.V.V.c, nil
		}
	}
	if cfg.F_get_config_for_client.Some {
		f := cfg.F_get_config_for_client.V
		c.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			r := f(tk, chiOf(chi))
			if !r.Ok {
				return nil, hookErr(r.E, "tls: get_config_for_client failed")
			}
			if !r.V.Some {
				return nil, nil
			}
			return serverConfig(tk, r.V.V), nil
		}
	}
	return c
}

// chiOf is Go's ClientHelloInfo as Xo's.
func chiOf(chi *tls.ClientHelloInfo) Tls_ClientHelloInfo {
	var cs, curves, points, schemes, vers, exts []int64
	for _, x := range chi.CipherSuites {
		cs = append(cs, int64(x))
	}
	for _, x := range chi.SupportedCurves {
		curves = append(curves, int64(x))
	}
	for _, x := range chi.SupportedPoints {
		points = append(points, int64(x))
	}
	for _, x := range chi.SignatureSchemes {
		schemes = append(schemes, int64(x))
	}
	for _, x := range chi.SupportedVersions {
		vers = append(vers, int64(x))
	}
	for _, x := range chi.Extensions {
		exts = append(exts, int64(x))
	}
	var la, ra string
	if chi.Conn != nil {
		la, ra = chi.Conn.LocalAddr().String(), chi.Conn.RemoteAddr().String()
	}
	return Tls_ClientHelloInfo{F_cipher_suites: ListFromSlice(cs), F_server_name: chi.ServerName, F_supported_curves: ListFromSlice(curves), F_supported_points: ListFromSlice(points), F_signature_schemes: ListFromSlice(schemes), F_supported_protos: ListFromSlice(append([]string(nil), chi.SupportedProtos...)), F_supported_versions: ListFromSlice(vers), F_extensions: ListFromSlice(exts), F_local_addr: la, F_remote_addr: ra}
}

// M_supports_certificate is `i.supports_certificate(cert)`: Go's
// ClientHelloInfo.SupportsCertificate under the default Config.
func (v Tls_ClientHelloInfo) M_supports_certificate(tk *Task, cert Tls_TlsCert) Result[Unit, Tls_TlsErr] {
	chi := &tls.ClientHelloInfo{ServerName: v.F_server_name, SupportedProtos: v.F_supported_protos.s}
	for _, x := range v.F_cipher_suites.s {
		chi.CipherSuites = append(chi.CipherSuites, uint16(x))
	}
	for _, x := range v.F_supported_curves.s {
		chi.SupportedCurves = append(chi.SupportedCurves, tls.CurveID(x))
	}
	for _, x := range v.F_supported_points.s {
		chi.SupportedPoints = append(chi.SupportedPoints, uint8(x))
	}
	for _, x := range v.F_signature_schemes.s {
		chi.SignatureSchemes = append(chi.SignatureSchemes, tls.SignatureScheme(x))
	}
	for _, x := range v.F_supported_versions.s {
		chi.SupportedVersions = append(chi.SupportedVersions, uint16(x))
	}
	if err := chi.SupportsCertificate(cert.c); err != nil {
		return Err[Unit](Tls_TlsErr{F_msg: err.Error()})
	}
	return Ok[Unit, Tls_TlsErr](Unit{})
}

// Tls_server is `tls.server(c, config)`: Go's tls.Server.
func Tls_server(tk *Task, c ServerConn, cfg Tls_ServerConfig) Tls_TlsServerConn {
	if c.c != nil {
		upgradedConns.Store(c.c, struct{}{})
	}
	if _, ok := c.c.(fakeConn); ok || c.c == nil {
		return Tls_TlsServerConn{Tls_TlsConn: Tls_TlsConn{c: c.c, fake: c.c != nil, closed: new(atomic.Bool)}}
	}
	t := tls.Server(c.c, serverConfig(tk, cfg))
	return Tls_TlsServerConn{Tls_TlsConn: Tls_TlsConn{t: t, c: c.c, closed: new(atomic.Bool)}, noCerts: len(cfg.F_certificates.s) == 0 && !cfg.F_get_certificate.Some && !cfg.F_get_config_for_client.Some}
}

// M_handshake is `t.handshake()`: Go's Conn.Handshake under the
// connection's deadlines.
func (t Tls_TlsServerConn) M_handshake(tk *Task) Result[Unit, NetErr] {
	if tk.Canceled() {
		return Err[Unit](netErr(5, ""))
	}
	if t.fake {
		return Ok[Unit, NetErr](Unit{})
	}
	if t.closed.Load() {
		return Err[Unit](netErr(4, netClosedText))
	}
	limit := userDeadline(t.c, false)
	if w := userDeadline(t.c, true); !w.IsZero() && (limit.IsZero() || w.Before(limit)) {
		limit = w
	}
	canceled, err := interruptibleTo(tk, t.c.SetDeadline, limit, t.t.Handshake)
	t.c.SetReadDeadline(userDeadline(t.c, false))
	t.c.SetWriteDeadline(userDeadline(t.c, true))
	if canceled {
		return Err[Unit](netErr(5, ""))
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return Err[Unit](netErr(3, ""))
	}
	if err != nil {
		return Err[Unit](netErr(4, err.Error()))
	}
	return Ok[Unit, NetErr](Unit{})
}

// M_read is `t.read(max)`: the handshake first (its deadline the
// connection's), then TlsConn's read.
func (t Tls_TlsServerConn) M_read(tk *Task, max int64) Result[Option[Bytes], NetErr] {
	if r := t.M_handshake(tk); !r.Ok {
		return Err[Option[Bytes]](r.E)
	}
	return t.Tls_TlsConn.M_read(tk, max)
}

// M_write is `t.write(data)`, after the handshake.
func (t Tls_TlsServerConn) M_write(tk *Task, data Bytes) Result[Unit, NetErr] {
	if r := t.M_handshake(tk); !r.Ok {
		return r
	}
	return t.Tls_TlsConn.M_write(tk, data)
}

// M_state is `t.state()`: the zero state before the handshake.
func (t Tls_TlsServerConn) M_state(tk *Task) Tls_ConnState {
	if t.t != nil && !t.t.ConnectionState().HandshakeComplete {
		return Tls_ConnState{F_negotiated_protocol_is_mutual: true}
	}
	return t.Tls_TlsConn.M_state(tk)
}

// M_net_conn is `t.net_conn()`.
// M_export_keying_material is TlsConn's (decision 0644 T9).
func (t Tls_TlsServerConn) M_export_keying_material(tk *Task, label string, context Option[Bytes], length int64) Result[Bytes, Tls_TlsErr] {
	return t.Tls_TlsConn.M_export_keying_material(tk, label, context, length)
}

func (t Tls_TlsServerConn) M_net_conn(*Task) ServerConn { return ServerConn{c: t.c} }

// Tls_listen is `tls.listen(net, addr, config)`: Go's tls.Listen.
func Tls_listen(tk *Task, n Net, addr string, cfg Tls_ServerConfig) Result[Tls_TlsListener, NetErr] {
	if len(cfg.F_certificates.s) == 0 && !cfg.F_get_certificate.Some && !cfg.F_get_config_for_client.Some {
		return Err[Tls_TlsListener](netErr(4, "tls: neither Certificates, GetCertificate, nor GetConfigForClient set in Config"))
	}
	l := n.M_listen(tk, addr)
	if !l.Ok {
		return Err[Tls_TlsListener](l.E)
	}
	return Ok[Tls_TlsListener, NetErr](Tls_TlsListener{l: l.V, cfg: cfg})
}

// Tls_new_listener is `tls.new_listener(l, config)`: Go's tls.NewListener.
func Tls_new_listener(_ *Task, l Listener, cfg Tls_ServerConfig) Tls_TlsListener {
	return Tls_TlsListener{l: l, cfg: cfg}
}

// M_accept is `l.accept()`: the next connection, its handshake not yet run.
func (l Tls_TlsListener) M_accept(tk *Task) Result[Tls_TlsServerConn, NetErr] {
	c := l.l.M_accept(tk)
	if !c.Ok {
		return Err[Tls_TlsServerConn](c.E)
	}
	return Ok[Tls_TlsServerConn, NetErr](Tls_server(tk, c.V, l.cfg))
}

// M_addr is `l.addr()`.
func (l Tls_TlsListener) M_addr(tk *Task) string { return l.l.M_addr(tk) }

// M_close is `l.close()`.
func (l Tls_TlsListener) M_close(tk *Task) { l.l.M_close(tk) }

// setPrefs sets decision 0644 T7's MinVersion, MaxVersion, CipherSuites and
// CurvePreferences as the native backends read them: TLS 1.2 is the least
// version (the native code has no 1.0 or 1.1), and suites and curves the
// native code does not implement are left out.
func setPrefs(c *tls.Config, minV, maxV int64, suites, curves List[int64]) {
	if minV != 0 {
		c.MinVersion = uint16(max(minV, tls.VersionTLS12))
	}
	if maxV != 0 {
		c.MaxVersion = uint16(maxV)
	}
	known := map[uint16]bool{}
	for _, cs := range tls.CipherSuites() {
		known[cs.ID] = true
	}
	for _, id := range suites.s {
		if known[uint16(id)] {
			c.CipherSuites = append(c.CipherSuites, uint16(id))
		}
	}
	if len(suites.s) != 0 && len(c.CipherSuites) == 0 {
		// none the native code runs: TLS 1.2 offers nothing
		c.CipherSuites = []uint16{}
	}
	for _, id := range curves.s {
		switch tls.CurveID(id) {
		case tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024, tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521:
			c.CurvePreferences = append(c.CurvePreferences, tls.CurveID(id))
		}
	}
}

// Tls_cipher_suites is Go's CipherSuites (every one the native code runs).
func Tls_cipher_suites(_ *Task) List[Tls_CipherSuite] {
	var out []Tls_CipherSuite
	for _, cs := range tls.CipherSuites() {
		vs := make([]int64, len(cs.SupportedVersions))
		for i, v := range cs.SupportedVersions {
			vs[i] = int64(v)
		}
		out = append(out, Tls_CipherSuite{F_id: int64(cs.ID), F_name: cs.Name, F_supported_versions: ListOf(vs...), F_insecure: cs.Insecure})
	}
	return ListOf(out...)
}

// Tls_insecure_cipher_suites is empty: the native code runs none of Go's
// InsecureCipherSuites.
func Tls_insecure_cipher_suites(_ *Task) List[Tls_CipherSuite] { return List[Tls_CipherSuite]{} }
