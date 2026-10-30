package rt

// std/tls on the Go backend (spec/std.md 27, decision 0281): crypto/tls's
// client on a connection the program dialed. std/native/tls.xo is the
// native backends'.

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
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

// Tls_client is `tls.client(c, host, roots, verify, alpn)`: the handshake
// on c (at most 10 seconds), which std/tls owns from then on.
func Tls_client(tk *Task, c Conn, host string, roots Option[Tls_Roots], verify Tls_Verify, alpn List[string]) Result[Tls_TlsConn, NetErr] {
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
	if roots.Some {
		cfg.RootCAs = roots.V.pool
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
