package rt

// std/tls on the Go backend (spec/std.md 27, decision 0281): crypto/tls's
// client on a connection the program dialed. std/native/tls.xo is the
// native backends'.

import (
	cryptorand "crypto/rand"
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
	st     *tlsConnSt
}

// tlsConnSt is a connection's state behind its lock (decision 0644
// T12): a lazy client's (Tls_client_conn) handshake, run once, and its
// Err; every connection's first failure, typed.
type tlsConnSt struct {
	mu    sync.Mutex
	err   *Tls_TlsErr
	lazy  bool
	ran   bool
	herr  *NetErr
	early *NetErr
}

// note records err (the text msg) as the connection's first failure.
func (s *tlsConnSt) note(msg string, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		e := tlsErrOf(msg, err)
		s.err = &e
	}
}

// tlsErrOf is err as a TlsErr with text msg and Go's error type as its
// kind (decision 0644 T12).
func tlsErrOf(msg string, err error) Tls_TlsErr {
	e := Tls_TlsErr{F_msg: msg, F_kind: Tls_TlsErrKind{Tag: 1}}
	var cv *tls.CertificateVerificationError
	var rh tls.RecordHeaderError
	var er *tls.ECHRejectionError
	var ae tls.AlertError
	switch {
	case errors.As(err, &ae):
		// decision 0644 T14 (QUIC)
		e.F_kind = Tls_TlsErrKind{Tag: 5, P: Tls_TlsErrKind_Alert{F_code: int64(ae)}}
	case errors.As(err, &er):
		// decision 0644 T13
		e.F_kind = Tls_TlsErrKind{Tag: 4, P: Tls_TlsErrKind_EchRejection{F_retry_config_list: Bytes(er.RetryConfigList)}}
	case errors.As(err, &cv):
		var certs []X509_Certificate
		for _, c := range cv.UnverifiedCertificates {
			certs = append(certs, x509CertOf(nil, c))
		}
		e.F_kind = Tls_TlsErrKind{Tag: 3, P: Tls_TlsErrKind_CertificateVerification{F_certs: ListFromSlice(certs), F_cause: x509ErrOf(cv.Err)}}
	case errors.As(err, &rh):
		e.F_kind = Tls_TlsErrKind{Tag: 2, P: Tls_TlsErrKind_RecordHeader{F_header: Bytes(rh.RecordHeader[:])}}
	}
	return e
}

// M_err is `t.err()`: the connection's first failure, typed.
func (t Tls_TlsConn) M_err(*Task) Option[Tls_TlsErr] {
	if t.st == nil {
		return Option[Tls_TlsErr]{}
	}
	t.st.mu.Lock()
	defer t.st.mu.Unlock()
	if t.st.err == nil {
		return Option[Tls_TlsErr]{}
	}
	return Some(*t.st.err)
}

// M_handshake is `t.handshake()`: a lazy client's handshake, run once
// (its Err again after); Ok on any other.
func (t Tls_TlsConn) M_handshake(tk *Task) Result[Unit, NetErr] {
	if t.st == nil || !t.st.lazy {
		return Ok[Unit, NetErr](Unit{})
	}
	t.st.mu.Lock()
	if t.st.ran {
		defer t.st.mu.Unlock()
		if t.st.herr != nil {
			return Err[Unit](*t.st.herr)
		}
		return Ok[Unit, NetErr](Unit{})
	}
	t.st.ran = true
	if e := t.st.early; e != nil {
		t.st.herr = e
		t.st.mu.Unlock()
		if e.Tag == 4 {
			t.st.note(e.P.(NetErr_Io).F_msg, errors.New(e.P.(NetErr_Io).F_msg))
		}
		return Err[Unit](*e)
	}
	t.st.mu.Unlock()
	if t.fake {
		return Ok[Unit, NetErr](Unit{})
	}
	r := tlsHandshake(tk, t)
	t.st.mu.Lock()
	if !r.Ok {
		e := r.E
		t.st.herr = &e
	}
	t.st.mu.Unlock()
	return r
}

// Tls_client_conn is `tls.client_conn(c, config)`: Go's tls.Client, the
// handshake at the first handshake, read or write.
func Tls_client_conn(tk *Task, c Conn, cfg Tls_ClientConfig) Tls_TlsConn {
	t, early := tlsClientConfigPrep(tk, c, cfg)
	if t.st == nil {
		t.st = new(tlsConnSt)
	}
	if t.closed == nil {
		t.closed = new(atomic.Bool)
	}
	t.st.lazy = true
	t.st.early = early
	return t
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
	t, early := tlsPrep(tk, c, host, roots, verify, alpn, cert, verifyConn, minVersion, maxVersion, cipherSuites, curvePrefs, extra)
	if early != nil {
		return Err[Tls_TlsConn](*early)
	}
	if t.fake {
		return Ok[Tls_TlsConn, NetErr](t)
	}
	if r := tlsHandshake(tk, t); !r.Ok {
		return Err[Tls_TlsConn](r.E)
	}
	return Ok[Tls_TlsConn, NetErr](t)
}

// tlsPrep is tlsClient's connection before its handshake, or the Err
// that stops it first.
func tlsPrep(tk *Task, c Conn, host string, roots Option[Tls_Roots], verify Tls_Verify, alpn List[string], cert Option[Tls_TlsCert], verifyConn Option[func(*Task, List[X509_Certificate]) Result[Unit, string]], minVersion int64, maxVersion int64, cipherSuites List[int64], curvePrefs List[int64], extra func(*tls.Config)) (Tls_TlsConn, *NetErr) {
	fail := func(e NetErr) (Tls_TlsConn, *NetErr) {
		return Tls_TlsConn{c: c.c, closed: new(atomic.Bool), st: new(tlsConnSt)}, &e
	}
	if tk.Canceled() {
		return fail(netErr(5, ""))
	}
	if c.c == nil {
		return fail(netErr(4, netClosedText))
	}
	if _, loaded := upgradedConns.LoadOrStore(c.c, struct{}{}); loaded {
		return fail(netErr(4, upgradedText))
	}
	if _, ok := c.c.(fakeConn); ok {
		return Tls_TlsConn{c: c.c, fake: true, closed: new(atomic.Bool), st: new(tlsConnSt)}, nil
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
	return Tls_TlsConn{t: tls.Client(c.c, cfg), c: c.c, closed: new(atomic.Bool), st: new(tlsConnSt)}, nil
}

// tlsHandshake is a client's handshake: http.DefaultTransport's
// TLSHandshakeTimeout, or the connection's own deadlines when earlier
// (decision 0430: Go's Handshake honors them); its failure is the
// connection's first (decision 0644 T12).
func tlsHandshake(tk *Task, t Tls_TlsConn) Result[Unit, NetErr] {
	limit := time.Now().Add(10 * time.Second)
	for _, w := range []bool{false, true} {
		if d := userDeadline(t.c, w); !d.IsZero() && d.Before(limit) {
			limit = d
		}
	}
	t.c.SetDeadline(limit)
	canceled, err := interruptible(tk, t.c.SetDeadline, t.t.Handshake)
	t.c.SetReadDeadline(userDeadline(t.c, false))
	t.c.SetWriteDeadline(userDeadline(t.c, true))
	if canceled {
		return Err[Unit](netErr(5, ""))
	}
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return Err[Unit](netErr(3, ""))
		}
		t.st.note(err.Error(), err)
		return Err[Unit](netErr(4, err.Error()))
	}
	return Ok[Unit, NetErr](Unit{})
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
	t, early := tlsClientConfigPrep(tk, c, cfg)
	if early != nil {
		return Err[Tls_TlsConn](*early)
	}
	if t.fake {
		return Ok[Tls_TlsConn, NetErr](t)
	}
	if r := tlsHandshake(tk, t); !r.Ok {
		return Err[Tls_TlsConn](r.E)
	}
	return Ok[Tls_TlsConn, NetErr](t)
}

// tlsClientConfigPrep is Tls_client_config's connection before its
// handshake.
func tlsClientConfigPrep(tk *Task, c Conn, cfg Tls_ClientConfig) (Tls_TlsConn, *NetErr) {
	var one Option[Tls_TlsCert]
	if len(cfg.F_certificates.s) == 1 {
		one = Option[Tls_TlsCert]{Some: true, V: cfg.F_certificates.s[0]}
	}
	return tlsPrep(tk, c, cfg.F_server_name, cfg.F_roots, cfg.F_verify, cfg.F_next_protos, one, Option[func(*Task, List[X509_Certificate]) Result[Unit, string]]{}, cfg.F_min_version, cfg.F_max_version, cfg.F_cipher_suites, cfg.F_curve_preferences, func(gc *tls.Config) {
		if len(cfg.F_certificates.s) > 1 {
			gc.Certificates = nil
			for _, x := range cfg.F_certificates.s {
				gc.Certificates = append(gc.Certificates, *x.c)
			}
		}
		tlsVerifyHooks(tk, gc, cfg.F_verify_peer_certificate, cfg.F_verify_connection, cfg.F_verify.Tag == 2)
		setKeyLog(tk, gc, cfg.F_key_log)
		// decision 0644 T13: Go's EncryptedClientHelloConfigList and
		// EncryptedClientHelloRejectionVerify
		if cfg.F_encrypted_client_hello_config_list.Some {
			gc.EncryptedClientHelloConfigList = append([]byte{}, cfg.F_encrypted_client_hello_config_list.V...)
		}
		if cfg.F_encrypted_client_hello_rejection_verify.Some {
			f := cfg.F_encrypted_client_hello_rejection_verify.V
			gc.EncryptedClientHelloRejectionVerify = func(cs tls.ConnectionState) error {
				if r := f(tk, connStateOf(tk, cs)); !r.Ok {
					return hookErr(r.E, "tls: encrypted_client_hello_rejection_verify failed")
				}
				return nil
			}
		}
		// decision 0644 T3: Go's ClientSessionCache and SessionTicketsDisabled
		gc.SessionTicketsDisabled = cfg.F_session_tickets_disabled
		if cfg.F_session_cache.Some {
			gc.ClientSessionCache = cfg.F_session_cache.V.c
		}
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
	if r := t.M_handshake(tk); !r.Ok {
		return Err[Option[Bytes]](r.E)
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
	e := netIoErr(err)
	t.st.note(e.P.(NetErr_Io).F_msg, err)
	return Err[Option[Bytes]](e)
}

// M_write is `t.write(data)`: writes every byte.
func (t Tls_TlsConn) M_write(tk *Task, data Bytes) Result[Unit, NetErr] {
	if tk.Canceled() {
		return Err[Unit](netErr(5, ""))
	}
	if r := t.M_handshake(tk); !r.Ok {
		return r
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
		e := netIoErr(err)
		t.st.note(e.P.(NetErr_Io).F_msg, err)
		return Err[Unit](e)
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
	l    Listener
	cfg  Tls_ServerConfig
	keys *autoTicketKeys
}

// Tls_SessionCache is a std/tls SessionCache (decision 0644 T3): Go's
// ClientSessionCache.
type Tls_SessionCache struct{ c tls.ClientSessionCache }

// XoDebug implements Debug.
func (Tls_SessionCache) XoDebug(b *strings.Builder) { b.WriteString("<sessioncache>") }

// Tls_lru_session_cache is `tls.lru_session_cache(capacity)`: Go's
// NewLRUClientSessionCache.
func Tls_lru_session_cache(_ *Task, capacity int64) Tls_SessionCache {
	return Tls_SessionCache{c: tls.NewLRUClientSessionCache(int(capacity))}
}

// Tls_TicketKey is a std/tls TicketKey (decision 0644 T4): one of Go's
// SetSessionTicketKeys keys.
type Tls_TicketKey struct{ k [32]byte }

// XoDebug implements Debug.
func (Tls_TicketKey) XoDebug(b *strings.Builder) { b.WriteString("<ticketkey>") }

// Tls_ticket_key is `tls.ticket_key(key)`: a key of 32 bytes.
func Tls_ticket_key(_ *Task, key Bytes) Result[Tls_TicketKey, Tls_TlsErr] {
	if len(key) != 32 {
		return Err[Tls_TicketKey](Tls_TlsErr{F_msg: fmt.Sprintf("tls: a session ticket key must be 32 bytes, not %d", len(key))})
	}
	var t Tls_TicketKey
	copy(t.k[:], key)
	return Ok[Tls_TicketKey, Tls_TlsErr](t)
}

// autoTicketKeys are a listener's automatic session ticket keys, as Go's
// Config.ticketKeys rotates them (a new key a day, each kept a week): a
// listener makes a Config per connection, so the keys live here.
type autoTicketKeys struct {
	mu   sync.Mutex
	keys [][32]byte
	made []time.Time
}

func (a *autoTicketKeys) current() [][32]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if len(a.keys) == 0 || now.Sub(a.made[0]) >= 24*time.Hour {
		var k [32]byte
		cryptorand.Read(k[:])
		a.keys = append([][32]byte{k}, a.keys...)
		a.made = append([]time.Time{now}, a.made...)
	}
	n := 0
	for n < len(a.made) && now.Sub(a.made[n]) < 7*24*time.Hour {
		n++
	}
	a.keys, a.made = a.keys[:n], a.made[:n]
	return append([][32]byte(nil), a.keys...)
}

// setTickets sets c's session tickets from cfg (decision 0644 T4): its
// keys, none when disabled, else auto's keys (a listener's), else none
// (a lone tls.server's keys would open no other connection's ticket).
func setTickets(c *tls.Config, cfg Tls_ServerConfig, auto *autoTicketKeys) {
	switch {
	case cfg.F_session_tickets_disabled:
		c.SessionTicketsDisabled = true
	case len(cfg.F_session_ticket_keys.s) > 0:
		var ks [][32]byte
		for _, k := range cfg.F_session_ticket_keys.s {
			ks = append(ks, k.k)
		}
		c.SetSessionTicketKeys(ks)
	case auto != nil:
		c.SetSessionTicketKeys(auto.current())
	default:
		c.SessionTicketsDisabled = true
	}
}

// XoDebug implements Debug.
func (Tls_TlsListener) XoDebug(b *strings.Builder) { b.WriteString("<tlslistener>") }

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

// serverConfig is cfg as crypto/tls's Config.
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
	// decision 0644 T13: Go's EncryptedClientHelloKeys and
	// GetEncryptedClientHelloKeys
	c.EncryptedClientHelloKeys = echKeysOf(cfg.F_encrypted_client_hello_keys)
	if cfg.F_get_encrypted_client_hello_keys.Some {
		g := cfg.F_get_encrypted_client_hello_keys.V
		c.GetEncryptedClientHelloKeys = func(chi *tls.ClientHelloInfo) ([]tls.EncryptedClientHelloKey, error) {
			r := g(tk, chiOf(chi))
			if !r.Ok {
				return nil, hookErr(r.E, "tls: get_encrypted_client_hello_keys failed")
			}
			ks := echKeysOf(r.V)
			if ks == nil {
				ks = []tls.EncryptedClientHelloKey{}
			}
			return ks, nil
		}
	}
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
			nc := serverConfig(tk, r.V.V)
			// Go's ticketKeys(configForClient): its own keys, or none
			// when disabled, else the first config's
			if r.V.V.F_session_tickets_disabled || len(r.V.V.F_session_ticket_keys.s) > 0 {
				setTickets(nc, r.V.V, nil)
			}
			return nc, nil
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
	return serverOn(tk, c, cfg, nil)
}

// serverOn is Tls_server with a listener's automatic ticket keys.
func serverOn(tk *Task, c ServerConn, cfg Tls_ServerConfig, auto *autoTicketKeys) Tls_TlsServerConn {
	if c.c != nil {
		upgradedConns.Store(c.c, struct{}{})
	}
	if _, ok := c.c.(fakeConn); ok || c.c == nil {
		return Tls_TlsServerConn{Tls_TlsConn: Tls_TlsConn{c: c.c, fake: c.c != nil, closed: new(atomic.Bool), st: new(tlsConnSt)}}
	}
	gc := serverConfig(tk, cfg)
	setTickets(gc, cfg, auto)
	t := tls.Server(c.c, gc)
	return Tls_TlsServerConn{Tls_TlsConn: Tls_TlsConn{t: t, c: c.c, closed: new(atomic.Bool), st: new(tlsConnSt)}, noCerts: len(cfg.F_certificates.s) == 0 && !cfg.F_get_certificate.Some && !cfg.F_get_config_for_client.Some}
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
		t.st.note(err.Error(), err)
		return Err[Unit](netErr(4, err.Error()))
	}
	return Ok[Unit, NetErr](Unit{})
}

// M_err is `t.err()`: the connection's first failure, typed (decision
// 0644 T12); with no certificate, the handshake's.
func (t Tls_TlsServerConn) M_err(tk *Task) Option[Tls_TlsErr] {
	if t.noCerts {
		return Some(Tls_TlsErr{F_msg: "tls: no certificates configured", F_kind: Tls_TlsErrKind{Tag: 1}})
	}
	return t.Tls_TlsConn.M_err(tk)
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
	return Ok[Tls_TlsListener, NetErr](Tls_TlsListener{l: l.V, cfg: cfg, keys: new(autoTicketKeys)})
}

// Tls_new_listener is `tls.new_listener(l, config)`: Go's tls.NewListener.
func Tls_new_listener(_ *Task, l Listener, cfg Tls_ServerConfig) Tls_TlsListener {
	return Tls_TlsListener{l: l, cfg: cfg, keys: new(autoTicketKeys)}
}

// M_accept is `l.accept()`: the next connection, its handshake not yet run.
func (l Tls_TlsListener) M_accept(tk *Task) Result[Tls_TlsServerConn, NetErr] {
	c := l.l.M_accept(tk)
	if !c.Ok {
		return Err[Tls_TlsServerConn](c.E)
	}
	return Ok[Tls_TlsServerConn, NetErr](serverOn(tk, c.V, l.cfg, l.keys))
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

// M_with_ocsp_staple is `c.with_ocsp_staple(staple)` (decision 0644
// T10): a copy of the certificate with Go's OCSPStaple.
func (c Tls_TlsCert) M_with_ocsp_staple(tk *Task, staple Bytes) Tls_TlsCert {
	n := *c.c
	n.OCSPStaple = []byte(staple)
	return Tls_TlsCert{c: &n}
}

// M_with_signed_certificate_timestamps is
// `c.with_signed_certificate_timestamps(scts)`: a copy with Go's
// SignedCertificateTimestamps.
func (c Tls_TlsCert) M_with_signed_certificate_timestamps(tk *Task, scts List[Bytes]) Tls_TlsCert {
	n := *c.c
	n.SignedCertificateTimestamps = nil
	for _, x := range scts.s {
		n.SignedCertificateTimestamps = append(n.SignedCertificateTimestamps, []byte(x))
	}
	return Tls_TlsCert{c: &n}
}

// echKeysOf is std/tls's ECH keys as crypto/tls's (decision 0644 T13).
func echKeysOf(keys List[Tls_EchKey]) []tls.EncryptedClientHelloKey {
	var out []tls.EncryptedClientHelloKey
	for _, k := range keys.s {
		out = append(out, tls.EncryptedClientHelloKey{Config: append([]byte{}, k.F_config...), PrivateKey: append([]byte{}, k.F_private_key...), SendAsRetry: k.F_send_as_retry})
	}
	return out
}
