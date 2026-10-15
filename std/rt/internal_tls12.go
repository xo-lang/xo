package rt

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// cryptoTLS12 is the reference of std/native/llrt/tls12.xo's ops: Go's
// own crypto/tls, run over recorded bytes. "client" drives tls.Client
// (TLS 1.2 only, no session tickets) against b's parts (3 byte lengths:
// the server's handshake records, its records after the handshake, the
// client's data, the root's DER) with d "time host suites groups alpn
// chunk"; its random source repeats the byte 0x11, so its random,
// session id, and ECDHE key are the trace's (a, the key the Xo client
// is given, is not needed here). The lines it returns are tls12.xo op's.
func cryptoTLS12(what string, a, b, c, d Bytes) (Bytes, bool) {
	if what != "client" {
		return "", false
	}
	parts := tls12Unpack([]byte(b))
	args := strings.Split(string(d), " ")
	if len(parts) != 4 || len(args) != 6 {
		return "args", true
	}
	// crypto/tls reads Config.Rand only with cryptocustomrand=1 (Go 1.26)
	old, had := os.LookupEnv("GODEBUG")
	os.Setenv("GODEBUG", "cryptocustomrand=1")
	defer func() {
		if had {
			os.Setenv("GODEBUG", old)
		} else {
			os.Unsetenv("GODEBUG")
		}
	}()
	now, _ := strconv.ParseInt(args[0], 10, 64)
	roots := x509.NewCertPool()
	if len(parts[3]) > 0 {
		root, err := x509.ParseCertificate(parts[3])
		if err != nil {
			return "parse", true
		}
		roots.AddCert(root)
	}
	cfg := &tls.Config{
		Rand:                   tls12Const(0x11),
		Time:                   func() time.Time { return time.Unix(now, 0).UTC() },
		RootCAs:                roots,
		MinVersion:             tls.VersionTLS12,
		MaxVersion:             tls.VersionTLS12,
		SessionTicketsDisabled: true,
	}
	if args[1] != "-" {
		cfg.ServerName = args[1]
	} else {
		cfg.InsecureSkipVerify = true
	}
	for _, s := range strings.Split(args[2], ",") {
		v, _ := strconv.ParseUint(s, 16, 16)
		cfg.CipherSuites = append(cfg.CipherSuites, uint16(v))
	}
	for _, g := range strings.Split(args[3], ",") {
		v, _ := strconv.Atoi(g)
		cfg.CurvePreferences = append(cfg.CurvePreferences, tls.CurveID(v))
	}
	if args[4] != "-" {
		cfg.NextProtos = strings.Split(args[4], ",")
	}
	chunk, _ := strconv.Atoi(args[5])
	conn := &tls12Conn{in: append(append([]byte(nil), parts[0]...), parts[1]...), chunk: chunk}
	cl := tls.Client(conn, cfg)
	err := cl.Handshake()
	out := "hs " + conn.take() + "\n"
	if err != nil {
		return Bytes(out + "result " + err.Error() + "\n"), true
	}
	st := cl.ConnectionState()
	proto := st.NegotiatedProtocol
	if proto == "" {
		proto = "-"
	}
	out += fmt.Sprintf("result ok %04x %d %s\n", st.CipherSuite, st.CurveID, proto)
	if len(parts[2]) > 0 {
		cl.Write(parts[2])
	}
	out += "app " + conn.take() + "\n"
	var got []byte
	buf := make([]byte, 4096)
	for {
		n, err := cl.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			out += "got " + hex.EncodeToString(got) + " " + conn.take() + " " + err.Error() + "\n"
			break
		}
	}
	cl.CloseWrite()
	out += "bye " + conn.take() + "\n"
	return Bytes(out), true
}

func tls12Unpack(b []byte) (parts [][]byte) {
	for len(b) >= 3 {
		n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
		parts = append(parts, b[3:3+n])
		b = b[3+n:]
	}
	return parts
}

// tls12Const is a random source repeating one byte: what crypto/tls
// reads from it does not depend on how much it reads.
type tls12Const byte

func (c tls12Const) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c)
	}
	return len(p), nil
}

// tls12Conn hands out recorded bytes (chunk at a time when chunk > 0,
// then EOF) and keeps what is written.
type tls12Conn struct {
	in    []byte
	chunk int
	out   bytes.Buffer
}

func (c *tls12Conn) Read(p []byte) (int, error) {
	if len(c.in) == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if c.chunk > 0 && n > c.chunk {
		n = c.chunk
	}
	n = copy(p[:n], c.in)
	c.in = c.in[n:]
	return n, nil
}

func (c *tls12Conn) Write(p []byte) (int, error) { return c.out.Write(p) }

func (c *tls12Conn) take() string {
	s := hex.EncodeToString(c.out.Bytes())
	c.out.Reset()
	return s
}

func (c *tls12Conn) Close() error                       { return nil }
func (c *tls12Conn) LocalAddr() net.Addr                { return nil }
func (c *tls12Conn) RemoteAddr() net.Addr               { return nil }
func (c *tls12Conn) SetDeadline(t time.Time) error      { return nil }
func (c *tls12Conn) SetReadDeadline(t time.Time) error  { return nil }
func (c *tls12Conn) SetWriteDeadline(t time.Time) error { return nil }
