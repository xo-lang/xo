package rt

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/mlkem"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// This file is std/native/llrt/tls13.xo and tlsrec.xo written out in Go
// with Go's crypto: the reference the tls13 crypto_op's goldens diff the
// Xo client against (work item B31). It follows the Xo file function for
// function, with crypto/x509's Verify for the certificate and the
// signature checks of crypto/ecdsa, crypto/rsa and crypto/ed25519.

// cryptoTLS13 is tls13.xo's op: `replay` (see its doc).
func cryptoTLS13(what string, a, b, c, d Bytes) (Bytes, bool) {
	if what == "groups" {
		// tls13.xo groups_op: what the HTTP client's hello offers
		// (http_net.go's noMLKEM; that file is not in every program)
		if os.Getenv("XO_TLS_MLKEM") == "0" {
			return "groups=1d,17,18,19 shares=1d", true
		}
		return "groups=11ec,11eb,11ed,1d,17,18,19 shares=11ec,1d", true
	}
	if what != "replay" {
		return "", false
	}
	var parts [][]byte
	for rest := []byte(b); len(rest) >= 3; {
		n := int(rest[0])<<16 | int(rest[1])<<8 | int(rest[2])
		parts = append(parts, rest[3:3+n])
		rest = rest[3+n:]
	}
	args := strings.Split(string(d), " ")
	if len(parts) != 4 || len(args) != 7 {
		return "args", true
	}
	cf := t13Conf{}
	if args[1] != "-" {
		cf.host = args[1]
	}
	cf.now, _ = strconv.ParseInt(args[0], 10, 64)
	if len(parts[3]) > 0 {
		root, err := x509.ParseCertificate(parts[3])
		if err != nil {
			return "parse", true
		}
		cf.roots = append(cf.roots, root)
	}
	for _, x := range strings.Split(args[2], ",") {
		v, _ := strconv.ParseUint(x, 16, 16)
		cf.suites = append(cf.suites, int(v))
	}
	for _, x := range strings.Split(args[5], ",") {
		v, _ := strconv.Atoi(x)
		cf.groups = append(cf.groups, v)
	}
	if args[6] != "-" {
		cf.alpn = strings.Split(args[6], ",")
	}
	switch args[4] {
	case "shared":
		cf.hello, cf.shared = []byte(a), []byte(c)
	case "key":
		cf.hello, cf.priv = []byte(a), []byte(c)
	default:
		cf.random, cf.sid, cf.priv = []byte(a[:32]), []byte(a[32:]), []byte(c)
	}
	chunk, _ := strconv.Atoi(args[3])
	cl := t13NewClient(cf)
	out := "ch " + hex.EncodeToString(cl.take()) + "\n"
	cl.feedIn(parts[0], chunk)
	out += fmt.Sprintf("flight %d %s %s\n", cl.state, hex.EncodeToString(cl.take()), cl.err)
	if cl.state == t13Done {
		alpn := cl.alpn
		if alpn == "" {
			alpn = "-"
		}
		out += fmt.Sprintf("params %s %s\n", t13SuiteName(cl.suite), alpn)
		cl.send(parts[2])
		out += "app " + hex.EncodeToString(cl.take()) + "\n"
		cl.feedIn(parts[1], chunk)
		app := cl.app
		cl.app = nil
		out += fmt.Sprintf("got %s closed %v %s %s\n", hex.EncodeToString(app), cl.closed, hex.EncodeToString(cl.take()), cl.err)
		cl.close()
		out += "bye " + hex.EncodeToString(cl.take()) + "\n"
	}
	return Bytes(out), true
}

const (
	t13WaitSH = iota
	t13WaitEE
	t13WaitCertCR
	t13WaitCert
	t13WaitCV
	t13WaitFin
	t13Done
	t13Failed
)

const t13X25519, t13P256, t13P384, t13P521, t13Hybrid = 29, 23, 24, 25, 0x11ec

// The other two hybrids of Go 1.26's defaults (the ECDH part first).
const t13HybridP256, t13HybridP384 = 0x11eb, 0x11ed

func t13IsHybrid(g int) bool { return g == t13Hybrid || g == t13HybridP256 || g == t13HybridP384 }

// t13HybridECDH is a hybrid's ECDH group.
func t13HybridECDH(g int) int {
	switch g {
	case t13HybridP256:
		return t13P256
	case t13HybridP384:
		return t13P384
	}
	return t13X25519
}

// t13MLKEM is a hybrid's ML-KEM key from its seed: the encapsulation
// key's bytes, the ciphertext size, and the decapsulation.
func t13MLKEM(g int, seed []byte) ([]byte, int, func([]byte) ([]byte, error)) {
	if g == t13HybridP384 {
		dk, err := mlkem.NewDecapsulationKey1024(seed)
		if err != nil {
			panic(err)
		}
		return dk.EncapsulationKey().Bytes(), mlkem.CiphertextSize1024, dk.Decapsulate
	}
	dk, err := mlkem.NewDecapsulationKey768(seed)
	if err != nil {
		panic(err)
	}
	return dk.EncapsulationKey().Bytes(), mlkem.CiphertextSize768, dk.Decapsulate
}

var t13Schemes = []int{0x0403, 0x0807, 0x0503, 0x0804, 0x0805, 0x0806, 0x0401, 0x0501, 0x0601, 0x0603}

type t13Conf struct {
	host                             string
	roots                            []*x509.Certificate
	now                              int64
	alpn                             []string
	suites, groups                   []int
	hello, random, sid, priv, shared []byte
}

type t13Client struct {
	conf                       t13Conf
	state                      int
	err, alpn                  string
	closed                     bool
	suite                      int
	peer                       []*x509.Certificate
	rin, hs, app, out, tr      []byte
	random, sid, priv, cookie  []byte
	shareGroup                 int
	hrr, ccsSent, certReq      bool
	reqCtx, hsSecret, cHS, sHS []byte
	rk, wk                     *t13Keys
}

func t13NewClient(cf t13Conf) *t13Client {
	c := &t13Client{conf: cf, rk: &t13Keys{}, wk: &t13Keys{}}
	ch := cf.hello
	if len(ch) == 0 {
		c.random, c.sid = cf.random, cf.sid
		if len(cf.groups) == 0 {
			c.fail(80, "tls: no supported elliptic curves for ECDHE")
			return c
		}
		c.shareGroup = cf.groups[0]
		c.priv = cf.priv
		ch = c.hello()
	} else if len(ch) > 38 {
		c.sid = ch[39 : 39+int(ch[38])]
		c.priv = cf.priv
		if len(cf.groups) > 0 {
			c.shareGroup = cf.groups[0]
		}
	}
	c.tr = append([]byte(nil), ch...)
	c.out = c.wk.seal(22, ch, 0x0301)
	return c
}

func (c *t13Client) take() []byte { o := c.out; c.out = nil; return o }

func (c *t13Client) send(data []byte) {
	if c.state == t13Done {
		c.out = append(c.out, c.wk.seal(23, data, 0x0303)...)
	}
}

func (c *t13Client) close() {
	if c.state != t13Failed {
		c.out = append(c.out, c.wk.seal(21, []byte{1, 0}, 0x0303)...)
	}
}

func (c *t13Client) feedIn(data []byte, chunk int) {
	if chunk <= 0 {
		c.feed(data)
		return
	}
	for i := 0; i < len(data); i += chunk {
		c.feed(data[i:min(i+chunk, len(data))])
	}
}

func (c *t13Client) feed(data []byte) {
	c.rin = append(c.rin, data...)
	for c.state != t13Failed && !c.closed && len(c.rin) >= 5 {
		if c.state == t13WaitSH && !c.hrr && ((c.rin[0] != 21 && c.rin[0] != 22) || c.rin[1] >= 16) {
			c.err, c.state = "tls: first record does not look like a TLS handshake", t13Failed
			return
		}
		n := int(c.rin[3])<<8 | int(c.rin[4])
		if n > 16640 {
			c.fail(22, fmt.Sprintf("tls: oversized record received with length %d", n))
			return
		}
		if len(c.rin) < 5+n {
			return
		}
		hdr, body := c.rin[:5:5], c.rin[5:5+n:5+n]
		c.rin = c.rin[5+n:]
		c.record(hdr, body)
	}
}

func (c *t13Client) record(hdr, body []byte) {
	ct := int(hdr[0])
	if ct == 20 {
		if c.state == t13Done || len(body) != 1 || body[0] != 1 {
			c.fail(10, "tls: unexpected message")
		}
		return
	}
	if c.rk.on() && ct != 23 {
		c.fail(10, "tls: unexpected message")
		return
	}
	ict, data := c.rk.open(hdr, body)
	switch {
	case ict == -1:
		c.fail(20, "local error: tls: bad record MAC")
	case ict == -2:
		c.fail(22, "local error: tls: record overflow")
	case ict == -3:
		c.fail(10, "local error: tls: unexpected message")
	case ict == 21:
		if len(data) != 2 {
			c.fail(10, "local error: tls: unexpected message")
			return
		}
		if data[1] == 0 {
			c.closed = true
			return
		}
		c.err = "remote error: tls: " + t13AlertText(int(data[1]))
		c.state = t13Failed
	case ict == 22:
		if len(data) == 0 {
			c.fail(10, "local error: tls: unexpected message")
			return
		}
		c.hs = append(c.hs, data...)
		for c.state != t13Failed && len(c.hs) >= 4 {
			t := int(c.hs[0])
			n := int(c.hs[1])<<16 | int(c.hs[2])<<8 | int(c.hs[3])
			most := 65536
			if t == 11 {
				most = 262144
			}
			if n > most {
				c.fail(80, fmt.Sprintf("tls: handshake message of length %d bytes exceeds maximum of %d bytes", n, most))
				return
			}
			if len(c.hs) < 4+n {
				return
			}
			msg := append([]byte(nil), c.hs[:4+n]...)
			c.hs = c.hs[4+n:]
			c.handshake(t, msg)
		}
	case ict == 23:
		if c.state != t13Done {
			c.fail(10, "local error: tls: unexpected message")
			return
		}
		c.app = append(c.app, data...)
	default:
		c.fail(10, "local error: tls: unexpected message")
	}
}

func (c *t13Client) handshake(t int, msg []byte) {
	switch {
	case c.state == t13WaitSH && t == 2:
		c.serverHello(msg)
	case c.state == t13WaitEE && t == 8:
		c.encryptedExtensions(msg)
	case c.state == t13WaitCertCR && t == 13:
		if len(msg) < 5 || len(msg) < 5+int(msg[4]) {
			c.fail(50, "tls: error decoding message")
			return
		}
		c.reqCtx = msg[5 : 5+int(msg[4])]
		c.certReq = true
		c.tr = append(c.tr, msg...)
		c.state = t13WaitCert
	case (c.state == t13WaitCertCR || c.state == t13WaitCert) && t == 11:
		c.certificate(msg)
	case c.state == t13WaitCV && t == 15:
		c.certificateVerify(msg)
	case c.state == t13WaitFin && t == 20:
		c.finished(msg)
	case c.state == t13Done && t == 4:
	case c.state == t13Done && t == 24:
		if len(msg) != 5 || msg[4] > 1 {
			c.fail(50, "tls: error decoding message")
			return
		}
		if len(c.hs) != 0 {
			c.fail(10, "tls: handshake buffer not empty before setting read traffic secret")
			return
		}
		c.rk = c.rk.update()
		if msg[4] == 1 {
			c.out = append(c.out, c.wk.seal(22, t13Message(24, []byte{0}), 0x0303)...)
			c.wk = c.wk.update()
		}
	default:
		c.fail(10, "tls: unexpected message")
	}
}

func (c *t13Client) hello() []byte {
	cf := c.conf
	var suites, exts []byte
	for _, s := range cf.suites {
		suites = t13U16(suites, s)
	}
	if cf.host != "" {
		if _, err := netip.ParseAddr(cf.host); err != nil {
			h := []byte(strings.TrimSuffix(cf.host, "."))
			exts = t13Ext(exts, 0, t13Vec16(append([]byte{0}, t13Vec16(h)...)))
		}
	}
	var groups, algs []byte
	for _, g := range cf.groups {
		groups = t13U16(groups, g)
	}
	exts = t13Ext(exts, 10, t13Vec16(groups))
	for _, a := range t13Schemes {
		algs = t13U16(algs, a)
	}
	exts = t13Ext(exts, 13, t13Vec16(algs))
	if len(cf.alpn) > 0 {
		var protos []byte
		for _, p := range cf.alpn {
			protos = append(protos, byte(len(p)))
			protos = append(protos, p...)
		}
		exts = t13Ext(exts, 16, t13Vec16(protos))
	}
	exts = t13Ext(exts, 43, []byte{2, 3, 4})
	if len(c.cookie) > 0 {
		exts = t13Ext(exts, 44, t13Vec16(c.cookie))
	}
	share := t13U16(nil, c.shareGroup)
	share = append(share, t13Vec16(t13PublicKey(c.shareGroup, c.priv))...)
	if len(c.t13SharedGroups()) == 2 {
		g := t13HybridECDH(c.shareGroup)
		share = t13U16(share, g)
		share = append(share, t13Vec16(t13PublicKey(g, c.priv[64:]))...)
	}
	exts = t13Ext(exts, 51, t13Vec16(share))
	body := []byte{3, 3}
	body = append(body, c.random...)
	body = append(body, byte(len(c.sid)))
	body = append(body, c.sid...)
	body = append(body, t13Vec16(suites)...)
	body = append(body, 1, 0)
	body = append(body, t13Vec16(exts)...)
	return t13Message(1, body)
}

type t13Hello struct {
	ok                          bool
	random, sid, key, cookie    []byte
	suite, comp, version, group int
}

func t13ParseHello(m []byte) (h t13Hello) {
	p := 4
	if len(m) < p+35 {
		return
	}
	h.version = t13U16At(m, p)
	h.random = m[p+2 : p+34]
	p += 34
	sn := int(m[p])
	if len(m) < p+1+sn+3 {
		return
	}
	h.sid = m[p+1 : p+1+sn]
	p += 1 + sn
	h.suite = t13U16At(m, p)
	h.comp = int(m[p+2])
	p += 3
	if p == len(m) {
		h.ok = true
		return
	}
	if len(m) < p+2 || p+2+t13U16At(m, p) != len(m) {
		return
	}
	p += 2
	seen := map[int]bool{}
	for p < len(m) {
		if len(m) < p+4 {
			return
		}
		t, n := t13U16At(m, p), t13U16At(m, p+2)
		if len(m) < p+4+n || seen[t] {
			return
		}
		seen[t] = true
		d := m[p+4 : p+4+n]
		switch t {
		case 43:
			if n != 2 {
				return
			}
			h.version = t13U16At(d, 0)
		case 51:
			if n < 2 {
				return
			}
			h.group = t13U16At(d, 0)
			if n > 2 {
				if n < 4 || t13U16At(d, 2) != n-4 {
					return
				}
				h.key = d[4:n]
			}
		case 44:
			if n < 2 || t13U16At(d, 0) != n-2 || n == 2 {
				return
			}
			h.cookie = d[2:n]
		}
		p += 4 + n
	}
	h.ok = true
	return
}

func t13Contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (c *t13Client) serverHello(msg []byte) {
	h := t13ParseHello(msg)
	if !h.ok {
		c.fail(50, "tls: error decoding message")
		return
	}
	if h.version != 0x0304 {
		c.fail(70, fmt.Sprintf("tls: server selected unsupported protocol version %x", h.version))
		return
	}
	if string(h.sid) != string(c.sid) {
		c.fail(47, "tls: server did not echo the legacy session ID")
		return
	}
	if h.comp != 0 {
		c.fail(47, "tls: server selected unsupported compression format")
		return
	}
	if !t13Contains(c.conf.suites, h.suite) || h.suite < 0x1301 || h.suite > 0x1303 || (c.hrr && h.suite != c.suite) {
		c.fail(47, "tls: server chose an unconfigured cipher suite")
		return
	}
	c.suite = h.suite
	hrrRandom := sha256.Sum256([]byte("HelloRetryRequest"))
	if string(h.random) == string(hrrRandom[:]) {
		c.retry(msg, h)
		return
	}
	if len(h.cookie) > 0 {
		c.fail(110, "tls: server sent a cookie in a normal ServerHello")
		return
	}
	shared := c.conf.shared
	if len(shared) == 0 {
		if !t13Contains(c.t13SharedGroups(), h.group) || len(h.key) == 0 {
			c.fail(47, "tls: server selected unsupported group")
			return
		}
		priv := c.priv
		if h.group != c.shareGroup {
			priv = c.priv[64:]
		}
		var ok bool
		if shared, ok = t13Shared(h.group, priv, h.key); !ok {
			c.fail(47, "tls: invalid server key share")
			return
		}
	}
	c.tr = append(c.tr, msg...)
	if len(c.hs) != 0 {
		c.fail(10, "tls: handshake buffer not empty before setting read traffic secret")
		return
	}
	hf, l := t13HashOf(c.suite)
	early, _ := hkdf.Extract(hf, make([]byte, l), nil)
	derived := t13ExpandLabel(hf, early, "derived", t13Sum(hf, nil), l)
	c.hsSecret, _ = hkdf.Extract(hf, shared, derived)
	th := t13Sum(hf, c.tr)
	c.cHS = t13ExpandLabel(hf, c.hsSecret, "c hs traffic", th, l)
	c.sHS = t13ExpandLabel(hf, c.hsSecret, "s hs traffic", th, l)
	c.dummyCCS()
	c.wk = t13NewKeys(c.suite, c.cHS)
	c.rk = t13NewKeys(c.suite, c.sHS)
	c.state = t13WaitEE
}

func (c *t13Client) retry(msg []byte, h t13Hello) {
	if c.hrr {
		c.fail(10, "tls: server sent two HelloRetryRequest messages")
		return
	}
	if len(c.conf.hello) > 0 {
		c.fail(80, "tls: HelloRetryRequest with a fixed ClientHello")
		return
	}
	if h.group == 0 && len(h.cookie) == 0 {
		c.fail(47, "tls: server sent an unnecessary HelloRetryRequest message")
		return
	}
	if h.group != 0 {
		if !t13Contains(c.conf.groups, h.group) {
			c.fail(47, "tls: server selected unsupported group")
			return
		}
		if t13Contains(c.t13SharedGroups(), h.group) {
			c.fail(47, "tls: server sent an unnecessary HelloRetryRequest key_share")
			return
		}
		c.shareGroup = h.group
		// the test hook's key (32 bytes) serves X25519 and P-256 alike
		c.priv = c.conf.priv
	}
	c.hrr = true
	c.cookie = h.cookie
	hf, _ := t13HashOf(c.suite)
	c.tr = append(t13Message(254, t13Sum(hf, c.tr)), msg...)
	c.dummyCCS()
	ch2 := c.hello()
	c.tr = append(c.tr, ch2...)
	c.out = append(c.out, c.wk.seal(22, ch2, 0x0303)...)
}

func (c *t13Client) dummyCCS() {
	if !c.ccsSent && len(c.sid) > 0 {
		c.ccsSent = true
		c.out = append(c.out, 20, 3, 3, 0, 1, 1)
	}
}

func (c *t13Client) encryptedExtensions(msg []byte) {
	if len(msg) < 6 || t13U16At(msg, 4) != len(msg)-6 {
		c.fail(50, "tls: error decoding message")
		return
	}
	for p := 6; p < len(msg); {
		if len(msg) < p+4 || len(msg) < p+4+t13U16At(msg, p+2) {
			c.fail(50, "tls: error decoding message")
			return
		}
		t, n := t13U16At(msg, p), t13U16At(msg, p+2)
		d := msg[p+4 : p+4+n]
		if t == 16 {
			if n < 3 || t13U16At(d, 0) != n-2 || int(d[2]) != n-3 {
				c.fail(50, "tls: error decoding message")
				return
			}
			proto := string(d[3:n])
			offered := false
			for _, a := range c.conf.alpn {
				offered = offered || a == proto
			}
			if !offered {
				c.fail(110, "tls: server advertised unrequested ALPN extension")
				return
			}
			c.alpn = proto
		}
		p += 4 + n
	}
	c.tr = append(c.tr, msg...)
	c.state = t13WaitCertCR
}

func (c *t13Client) certificate(msg []byte) {
	if len(msg) < 8 || msg[4] != 0 || t13U24At(msg, 5) != len(msg)-8 {
		c.fail(50, "tls: error decoding message")
		return
	}
	var certs []*x509.Certificate
	for p := 8; p < len(msg); {
		if len(msg) < p+3 || len(msg) < p+3+t13U24At(msg, p)+2 {
			c.fail(50, "tls: error decoding message")
			return
		}
		n := t13U24At(msg, p)
		der := msg[p+3 : p+3+n]
		p += 3 + n
		en := t13U16At(msg, p)
		if len(msg) < p+2+en {
			c.fail(50, "tls: error decoding message")
			return
		}
		p += 2 + en
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			c.fail(42, "tls: failed to parse certificate from server: "+err.Error())
			return
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		c.fail(50, "tls: received empty certificates message")
		return
	}
	switch k := certs[0].PublicKey.(type) {
	case *rsa.PublicKey, ed25519.PublicKey:
	case *ecdsa.PublicKey:
		// any curve, as crypto/tls (P-224's has no TLS 1.3 scheme)
		_ = k
	default:
		c.fail(43, "tls: server's certificate contains an unsupported type of public key")
		return
	}
	roots, inters := x509.NewCertPool(), x509.NewCertPool()
	for _, r := range c.conf.roots {
		roots.AddCert(r)
	}
	for _, x := range certs[1:] {
		inters.AddCert(x)
	}
	opts := x509.VerifyOptions{Roots: roots, Intermediates: inters, DNSName: c.conf.host,
		CurrentTime: time.Unix(c.conf.now, 0).UTC(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if _, err := certs[0].Verify(opts); err != nil {
		alert := 42
		var ua x509.UnknownAuthorityError
		var ci x509.CertificateInvalidError
		if errors.As(err, &ua) {
			alert = 48
		} else if errors.As(err, &ci) && ci.Reason == x509.Expired {
			alert = 45
		}
		c.fail(alert, "tls: failed to verify certificate: "+err.Error())
		return
	}
	c.peer = certs
	c.tr = append(c.tr, msg...)
	c.state = t13WaitCV
}

func (c *t13Client) certificateVerify(msg []byte) {
	if len(msg) < 8 || t13U16At(msg, 6) != len(msg)-8 {
		c.fail(50, "tls: error decoding message")
		return
	}
	scheme, sig := t13U16At(msg, 4), msg[8:]
	hf, _ := t13HashOf(c.suite)
	content := append([]byte(strings.Repeat(" ", 64)), "TLS 1.3, server CertificateVerify\x00"...)
	content = append(content, t13Sum(hf, c.tr)...)
	ok, known, why := false, true, ""
	switch k := c.peer[0].PublicKey.(type) {
	case *ecdsa.PublicKey:
		why = "ECDSA verification failure"
		switch {
		case scheme == 0x0403 && k.Curve.Params().BitSize == 256:
			d := sha256.Sum256(content)
			ok = ecdsa.VerifyASN1(k, d[:], sig)
		case scheme == 0x0503 && k.Curve.Params().BitSize == 384:
			d := sha512.Sum384(content)
			ok = ecdsa.VerifyASN1(k, d[:], sig)
		case scheme == 0x0603 && k.Curve.Params().BitSize == 521:
			d := sha512.Sum512(content)
			ok = ecdsa.VerifyASN1(k, d[:], sig)
		default:
			known = false
		}
	case *rsa.PublicKey:
		why = "crypto/rsa: verification error"
		var h crypto.Hash
		switch scheme {
		case 0x0804:
			h = crypto.SHA256
		case 0x0805:
			h = crypto.SHA384
		case 0x0806:
			h = crypto.SHA512
		default:
			known = false
		}
		if known {
			d := h.New()
			d.Write(content)
			ok = rsa.VerifyPSS(k, h, d.Sum(nil), sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) == nil
		}
	case ed25519.PublicKey:
		why = "Ed25519 verification failure"
		known = scheme == 0x0807
		ok = known && ed25519.Verify(k, content, sig)
	default:
		known = false
	}
	if !known || !t13Contains(t13Schemes, scheme) {
		c.fail(47, "tls: certificate used with invalid signature algorithm")
		return
	}
	if !ok {
		c.fail(51, "tls: invalid signature by the server certificate: "+why)
		return
	}
	c.tr = append(c.tr, msg...)
	c.state = t13WaitFin
}

func (c *t13Client) finished(msg []byte) {
	hf, l := t13HashOf(c.suite)
	if !hmac.Equal(msg[4:], t13VerifyData(hf, l, c.sHS, c.tr)) {
		c.fail(51, "tls: invalid server finished hash")
		return
	}
	c.tr = append(c.tr, msg...)
	if len(c.hs) != 0 {
		c.fail(10, "tls: handshake buffer not empty before setting read traffic secret")
		return
	}
	derived := t13ExpandLabel(hf, c.hsSecret, "derived", t13Sum(hf, nil), l)
	master, _ := hkdf.Extract(hf, make([]byte, l), derived)
	th := t13Sum(hf, c.tr)
	cAP := t13ExpandLabel(hf, master, "c ap traffic", th, l)
	c.rk = t13NewKeys(c.suite, t13ExpandLabel(hf, master, "s ap traffic", th, l))
	var flight []byte
	if c.certReq {
		cm := t13Message(11, append(append([]byte{byte(len(c.reqCtx))}, c.reqCtx...), 0, 0, 0))
		c.tr = append(c.tr, cm...)
		flight = cm
	}
	fin := t13Message(20, t13VerifyData(hf, l, c.cHS, c.tr))
	c.tr = append(c.tr, fin...)
	flight = append(flight, fin...)
	c.out = append(c.out, c.wk.seal(22, flight, 0x0303)...)
	c.wk = t13NewKeys(c.suite, cAP)
	c.state = t13Done
}

func (c *t13Client) fail(alert int, msg string) {
	if c.state != t13Failed {
		c.err, c.state = msg, t13Failed
		c.out = append(c.out, c.wk.seal(21, []byte{2, byte(alert)}, 0x0303)...)
	}
}

// t13Keys is tlsrec.xo's Keys: no suite before the handshake keys.
type t13Keys struct {
	suite           int
	secret, key, iv []byte
	gcm             cipher.AEAD
	seq             uint64
}

func t13NewKeys(suite int, secret []byte) *t13Keys {
	hf, _ := t13HashOf(suite)
	kl := 32
	if suite == 0x1301 {
		kl = 16
	}
	k := &t13Keys{suite: suite, secret: secret, key: t13ExpandLabel(hf, secret, "key", nil, kl), iv: t13ExpandLabel(hf, secret, "iv", nil, 12)}
	if suite != 0x1303 {
		blk, _ := aes.NewCipher(k.key)
		k.gcm, _ = cipher.NewGCM(blk)
	}
	return k
}

func (k *t13Keys) on() bool { return k.suite != 0 }

func (k *t13Keys) update() *t13Keys {
	hf, l := t13HashOf(k.suite)
	return t13NewKeys(k.suite, t13ExpandLabel(hf, k.secret, "traffic upd", nil, l))
}

func (k *t13Keys) nonce() []byte {
	n := append([]byte(nil), k.iv...)
	for i := 0; i < 8; i++ {
		n[4+i] ^= byte(k.seq >> (56 - 8*i))
	}
	return n
}

func (k *t13Keys) seal(ct int, data []byte, version int) []byte {
	var out []byte
	for off := 0; ; {
		end := min(off+16384, len(data))
		part := data[off:end]
		if !k.on() {
			out = append(out, byte(ct), byte(version>>8), byte(version), byte(len(part)>>8), byte(len(part)))
			out = append(out, part...)
		} else {
			inner := append(append([]byte(nil), part...), byte(ct))
			hdr := []byte{23, 3, 3, byte((len(part) + 17) >> 8), byte(len(part) + 17)}
			out = append(out, hdr...)
			if k.gcm != nil {
				out = k.gcm.Seal(out, k.nonce(), inner, hdr)
			} else {
				sealed := chachaXor(k.key, 1, k.nonce(), inner)
				out = append(out, sealed...)
				out = append(out, chachaTag(k.key, k.nonce(), sealed, hdr)...)
			}
			k.seq++
		}
		off = end
		if off >= len(data) {
			return out
		}
	}
}

// open is tlsrec.xo's open: the inner type, or -1 (bad MAC), -2 (too
// long), -3 (all padding).
func (k *t13Keys) open(hdr, body []byte) (int, []byte) {
	if !k.on() {
		return int(hdr[0]), body
	}
	var pt []byte
	if k.gcm != nil {
		var err error
		if pt, err = k.gcm.Open(nil, k.nonce(), body, hdr); err != nil {
			return -1, nil
		}
	} else {
		if len(body) < 16 {
			return -1, nil
		}
		ct := body[:len(body)-16]
		if !hmac.Equal(chachaTag(k.key, k.nonce(), ct, hdr), body[len(body)-16:]) {
			return -1, nil
		}
		pt = chachaXor(k.key, 1, k.nonce(), ct)
	}
	k.seq++
	if len(pt) > 16385 {
		return -2, nil
	}
	i := len(pt) - 1
	for i >= 0 && pt[i] == 0 {
		i--
	}
	if i < 0 {
		return -3, nil
	}
	return int(pt[i]), pt[:i]
}

func t13HashOf(suite int) (func() hash.Hash, int) {
	if suite == 0x1302 {
		return sha512.New384, 48
	}
	return sha256.New, 32
}

func t13Sum(hf func() hash.Hash, b []byte) []byte { h := hf(); h.Write(b); return h.Sum(nil) }

func t13ExpandLabel(hf func() hash.Hash, secret []byte, label string, context []byte, n int) []byte {
	info := []byte{byte(n >> 8), byte(n), byte(6 + len(label))}
	info = append(info, "tls13 "+label...)
	info = append(info, byte(len(context)))
	info = append(info, context...)
	out, _ := hkdf.Expand(hf, secret, string(info), n)
	return out
}

func t13VerifyData(hf func() hash.Hash, l int, base, tr []byte) []byte {
	m := hmac.New(hf, t13ExpandLabel(hf, base, "finished", nil, l))
	m.Write(t13Sum(hf, tr))
	return m.Sum(nil)
}

func t13Curve(group int) ecdh.Curve {
	switch group {
	case t13X25519:
		return ecdh.X25519()
	case t13P384:
		return ecdh.P384()
	case t13P521:
		return ecdh.P521()
	}
	return ecdh.P256()
}

// t13SharedGroups are the groups of the ClientHello's key shares: the
// first group's, and X25519 next to the hybrid (from its own key).
func (c *t13Client) t13SharedGroups() []int {
	// not after a HelloRetryRequest (Go: no fallback ECDH share then)
	if !c.hrr && t13IsHybrid(c.shareGroup) && t13Contains(c.conf.groups, t13HybridECDH(c.shareGroup)) {
		return []int{c.shareGroup, t13HybridECDH(c.shareGroup)}
	}
	return []int{c.shareGroup}
}

func t13PublicKey(group int, priv []byte) []byte {
	if t13IsHybrid(group) {
		ek, _, _ := t13MLKEM(group, priv[:64])
		e := t13PublicKey(t13HybridECDH(group), priv[64:])
		if group == t13Hybrid {
			return append(ek, e...)
		}
		return append(e, ek...)
	}
	k, err := t13Curve(group).NewPrivateKey(priv)
	if err != nil {
		panic(err)
	}
	return k.PublicKey().Bytes()
}

func t13Shared(group int, priv, peer []byte) ([]byte, bool) {
	if t13IsHybrid(group) {
		eg := t13HybridECDH(group)
		es := len(t13PublicKey(eg, priv[64:]))
		_, cs, decaps := t13MLKEM(group, priv[:64])
		if len(peer) != cs+es {
			return nil, false
		}
		eShare, ct := peer[:es], peer[es:]
		if group == t13Hybrid {
			ct, eShare = peer[:cs], peer[cs:]
		}
		x, ok := t13Shared(eg, priv[64:], eShare)
		if !ok {
			return nil, false
		}
		k, err := decaps(ct)
		if err != nil {
			return nil, false
		}
		if group == t13Hybrid {
			return append(k, x...), true
		}
		return append(x, k...), true
	}
	k, err := t13Curve(group).NewPrivateKey(priv)
	if err != nil {
		panic(err)
	}
	p, err := t13Curve(group).NewPublicKey(peer)
	if err != nil {
		return nil, false
	}
	s, err := k.ECDH(p)
	return s, err == nil
}

func t13U16(b []byte, x int) []byte { return append(b, byte(x>>8), byte(x)) }

func t13Vec16(b []byte) []byte { return append(t13U16(nil, len(b)), b...) }

func t13Ext(exts []byte, t int, d []byte) []byte { return append(t13U16(exts, t), t13Vec16(d)...) }

func t13Message(t int, body []byte) []byte {
	return append([]byte{byte(t), byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
}

func t13U16At(b []byte, p int) int { return int(b[p])<<8 | int(b[p+1]) }

func t13U24At(b []byte, p int) int { return int(b[p])<<16 | int(b[p+1])<<8 | int(b[p+2]) }

func t13SuiteName(s int) string {
	switch s {
	case 0x1301:
		return "TLS_AES_128_GCM_SHA256"
	case 0x1302:
		return "TLS_AES_256_GCM_SHA384"
	case 0x1303:
		return "TLS_CHACHA20_POLY1305_SHA256"
	}
	return fmt.Sprintf("0x%04X", s)
}

// t13AlertText is crypto/tls's alertText (tlsname.xo's alert_text).
func t13AlertText(a int) string {
	texts := map[int]string{0: "close notify", 10: "unexpected message", 20: "bad record MAC", 21: "decryption failed",
		22: "record overflow", 30: "decompression failure", 40: "handshake failure", 42: "bad certificate",
		43: "unsupported certificate", 44: "revoked certificate", 45: "expired certificate", 46: "unknown certificate",
		47: "illegal parameter", 48: "unknown certificate authority", 49: "access denied", 50: "error decoding message",
		51: "error decrypting message", 60: "export restriction", 70: "protocol version not supported",
		71: "insufficient security level", 80: "internal error", 86: "inappropriate fallback", 90: "user canceled",
		100: "no renegotiation", 109: "missing extension", 110: "unsupported extension", 111: "certificate unobtainable",
		112: "unrecognized name", 113: "bad certificate status response", 114: "bad certificate hash value",
		115: "unknown PSK identity", 116: "certificate required", 120: "no application protocol", 121: "encrypted client hello required"}
	if t, ok := texts[a]; ok {
		return t
	}
	return fmt.Sprintf("alert(%d)", a)
}
