package rt

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
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
// is given, is not needed here). "combined" is the client offering TLS
// 1.3 and 1.2 in one ClientHello (b's first part, the trace's, is the one
// crypto/tls builds itself here). The lines it returns are tls12.xo op's.
func cryptoTLS12(what string, a, b, c, d Bytes) (Bytes, bool) {
	if what == "server" {
		return tls12Server(b, d)
	}
	if what != "client" && what != "combined" {
		return "", false
	}
	parts := tls12Unpack([]byte(b))
	args := strings.Split(string(d), " ")
	if what == "combined" && len(parts) == 5 {
		parts = parts[1:]
	} else if what == "combined" {
		return "args", true
	}
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
	if what == "combined" {
		cfg.MaxVersion = tls.VersionTLS13
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

// tls12Server is tls12srv.xo op's reference: tls.Server (TLS 1.2 only,
// no session tickets) over b's parts (the client's handshake records,
// its records after the handshake, the leaf's DER, its key: PKCS #1 DER
// or an ECDSA scalar, and a signature the Xo server is given, not needed
// here) with d "suites groups alpn keytype chunk mask"; its random
// source repeats 0x22, so its random, ECDHE key, and RSA-PSS salts are
// the trace's.
func tls12Server(b, d Bytes) (Bytes, bool) {
	parts := tls12Unpack([]byte(b))
	args := strings.Split(string(d), " ")
	if len(parts) != 5 || (len(args) != 6 && len(args) != 7) {
		return "args", true
	}
	old, had := os.LookupEnv("GODEBUG")
	os.Setenv("GODEBUG", "cryptocustomrand=1")
	defer func() {
		if had {
			os.Setenv("GODEBUG", old)
		} else {
			os.Unsetenv("GODEBUG")
		}
	}()
	var key crypto.Signer
	var err error
	switch args[3] {
	case "rsa":
		key, err = x509.ParsePKCS1PrivateKey(parts[3])
	case "p384":
		key, err = ecdsa.ParseRawPrivateKey(elliptic.P384(), parts[3])
	default:
		key, err = ecdsa.ParseRawPrivateKey(elliptic.P256(), parts[3])
	}
	if err != nil {
		return "key", true
	}
	cfg := &tls.Config{
		Rand:                   tls12Const(0x22),
		Certificates:           []tls.Certificate{{Certificate: [][]byte{parts[2]}, PrivateKey: key}},
		MinVersion:             tls.VersionTLS12,
		MaxVersion:             tls.VersionTLS12,
		SessionTicketsDisabled: true,
	}
	for _, s := range strings.Split(args[0], ",") {
		v, _ := strconv.ParseUint(s, 16, 16)
		cfg.CipherSuites = append(cfg.CipherSuites, uint16(v))
	}
	for _, g := range strings.Split(args[1], ",") {
		v, _ := strconv.Atoi(g)
		cfg.CurvePreferences = append(cfg.CurvePreferences, tls.CurveID(v))
	}
	if args[2] != "-" {
		cfg.NextProtos = strings.Split(args[2], ",")
	}
	if len(args) == 7 && args[6] != "-" {
		// session tickets, at that time (the automatic ticket key comes
		// from the random source too)
		now, _ := strconv.ParseInt(args[6], 10, 64)
		cfg.SessionTicketsDisabled = false
		cfg.Time = func() time.Time { return time.Unix(now, 0).UTC() }
	}
	chunk, _ := strconv.Atoi(args[4])
	conn := &tls12Conn{in: append(append([]byte(nil), parts[0]...), parts[1]...), chunk: chunk}
	sv := tls.Server(conn, cfg)
	err = sv.Handshake()
	raw := append([]byte(nil), conn.out.Bytes()...)
	hs := conn.take()
	if args[5] == "1" {
		cr := bytes.Repeat([]byte{0x11}, 32)
		if in := parts[0]; len(in) >= 43 && in[0] == 22 && in[5] == 1 {
			cr = in[11:43]
		} else if all := append(append([]byte(nil), parts[0]...), parts[1]...); len(all) >= 43 && all[0] == 22 && all[5] == 1 {
			cr = all[11:43]
		}
		hs = tls12MaskSig(raw, parts[2], cr)
	}
	out := "hs " + hs + "\n"
	if err != nil {
		return Bytes(out + "result " + err.Error() + "\n"), true
	}
	st := sv.ConnectionState()
	proto := st.NegotiatedProtocol
	if proto == "" {
		proto = "-"
	}
	resumed := ""
	if len(args) == 7 {
		resumed = fmt.Sprintf(" resumed=%v", st.DidResume)
	}
	out += fmt.Sprintf("result ok %04x %d %s%s\n", st.CipherSuite, st.CurveID, proto, resumed)
	var got []byte
	buf := make([]byte, 4096)
	for {
		n, err := sv.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			out += "got " + hex.EncodeToString(got) + " " + conn.take() + " " + err.Error() + "\n"
			break
		}
	}
	sv.Write(append([]byte("pong:"), got...))
	out += "app " + conn.take() + "\n"
	sv.Close()
	out += "bye " + conn.take() + "\n"
	return Bytes(out), true
}

// tls12MaskSig is tls12srv.xo mask_sig: the hex of the server's records
// with the ServerKeyExchange's record replaced by "<ske-ok>" or
// "<ske-bad>", its signature checked against the leaf's key over the
// client random cr.
func tls12MaskSig(out, leafDER, cr []byte) string {
	var b strings.Builder
	var sr []byte
	for len(out) >= 5 && out[0] == 22 && 5+(int(out[3])<<8|int(out[4])) <= len(out) {
		n := int(out[3])<<8 | int(out[4])
		body := out[5 : 5+n]
		if len(body) > 38 && body[0] == 2 {
			sr = body[6:38]
		}
		if len(body) > 8 && body[0] == 12 {
			k := body[4:]
			pl := int(k[3])
			ok := false
			if len(k) >= 8+pl {
				if leaf, err := x509.ParseCertificate(leafDER); err == nil {
					signed := append(append(append([]byte(nil), cr...), sr...), k[:4+pl]...)
					ok = tls12Verify(int(k[4+pl])<<8|int(k[5+pl]), leaf.PublicKey, signed, k[8+pl:])
				}
			}
			if ok {
				b.WriteString("<ske-ok>")
			} else {
				b.WriteString("<ske-bad>")
			}
		} else {
			b.WriteString(hex.EncodeToString(out[:5+n]))
		}
		out = out[5+n:]
	}
	b.WriteString(hex.EncodeToString(out))
	return b.String()
}

func tls12Verify(scheme int, pub any, signed, sig []byte) bool {
	var h crypto.Hash
	var digest []byte
	switch scheme & 0xff00 {
	case 0x0400, 0x0800:
		h = crypto.SHA256
	case 0x0500:
		h = crypto.SHA384
	case 0x0600:
		h = crypto.SHA512
	}
	if scheme == 0x0805 {
		h = crypto.SHA384
	} else if scheme == 0x0806 {
		h = crypto.SHA512
	}
	switch h {
	case crypto.SHA256:
		s := sha256.Sum256(signed)
		digest = s[:]
	case crypto.SHA384:
		s := sha512.Sum384(signed)
		digest = s[:]
	default:
		s := sha512.Sum512(signed)
		digest = s[:]
	}
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		return ecdsa.VerifyASN1(k, digest, sig)
	case *rsa.PublicKey:
		if scheme>>8 == 8 {
			return rsa.VerifyPSS(k, h, digest, sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) == nil
		}
		return rsa.VerifyPKCS1v15(k, h, digest, sig) == nil
	}
	return false
}
