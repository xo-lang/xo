package rt

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/mlkem"
	"crypto/mlkem/mlkemtest"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// This file is std/native/llrt/tls13srv.xo written out in Go with Go's
// crypto (over internal_tls13.go's records and key schedule): the
// reference the tls13srv crypto_op's goldens diff the Xo server against
// (work item B31). It follows the Xo file function for function (less
// Conf.alts and HANDOFF, which only tlssrv.xo's connections use); `pair`
// runs Go's crypto/tls client against its server instead.

// cryptoTLS13Srv is tls13srv.xo's op: `replay` and `pair` (see its doc).
func cryptoTLS13Srv(what string, a, b, c, d Bytes) (Bytes, bool) {
	args := strings.Split(string(d), " ")
	if what == "replay" && len(args) == 6 {
		return Bytes(t13sReplay(t13sUnpack([]byte(a)), t13sUnpack([]byte(b)), t13sUnpack([]byte(c)), args)), true
	}
	if what == "pair" && len(args) == 5 {
		return Bytes(t13sPair([]byte(a), []byte(b), args)), true
	}
	return "", false
}

func t13sUnpack(b []byte) (parts [][]byte) {
	for len(b) >= 3 {
		n := t13U24At(b, 0)
		parts = append(parts, b[3:3+n])
		b = b[3+n:]
	}
	return parts
}

func t13sInts(s string, base int) (out []int) {
	for _, x := range strings.Split(s, ",") {
		v, _ := strconv.ParseInt(x, base, 32)
		out = append(out, int(v))
	}
	return out
}

func t13sDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func t13sReplay(flights, sv, hooks [][]byte, args []string) string {
	ncerts, _ := strconv.Atoi(args[5])
	if len(sv) != ncerts+4 || (len(hooks) != 3 && len(hooks) != 7 && len(hooks) != 8) {
		return "args\n"
	}
	cf := t13sConf{certs: sv[:ncerts], key: sv[ncerts], suites: []int{0x1301, 0x1302, 0x1303}, groups: []int{0x11ec, t13HybridP256, t13HybridP384, t13X25519, t13P256, t13P384, t13P521}}
	if args[0] != "-" {
		cf.suites = t13sInts(args[0], 16)
	}
	if args[1] != "-" {
		cf.groups = t13sInts(args[1], 10)
	}
	if args[2] != "-" {
		cf.alpn = strings.Split(args[2], ",")
	}
	cf.rfc = args[3] == "1"
	if string(sv[ncerts+1]) != "-" {
		cf.ee, cf.hasEE = sv[ncerts+1], true
	}
	cf.ticket = sv[ncerts+2]
	cf.random, cf.priv, cf.salt = hooks[0], hooks[1], hooks[2]
	if len(hooks) == 8 {
		cf.mlkemM = hooks[7]
	}
	if len(hooks) >= 7 && len(hooks[3]) > 0 {
		cf.now, _ = strconv.ParseInt(string(hooks[4]), 10, 64)
		cf.ticketKeys = []t13sTicketKey{t13sKeyFrom(hooks[3], cf.now)}
		cf.ticketIV, cf.ageAdd = hooks[5], hooks[6]
	}
	chunk, _ := strconv.Atoi(args[4])
	s := &t13Server{conf: cf}
	out := ""
	for i, f := range flights {
		was := s.state
		s.feedIn(f, chunk)
		if was != t13sDone && s.state == t13sDone {
			s.send(sv[ncerts+3])
		}
		app := s.app
		s.app = nil
		out += fmt.Sprintf("f%d %d %s %s %v %s\n", i, s.state, t13sDash(hex.EncodeToString(s.take())), t13sDash(hex.EncodeToString(app)), s.closed, s.err)
		if was != t13sDone && s.state == t13sDone {
			out += fmt.Sprintf("params %s %d %s %x %s %v %v\n", t13SuiteName(s.suite), s.group, t13sDash(s.alpn), s.scheme, t13sDash(s.sni), s.hrr, s.resumed)
		}
	}
	s.close()
	return out + "bye " + t13sDash(hex.EncodeToString(s.take())) + "\n"
}

const (
	t13sWaitCH = iota
	t13sWaitCH2
	t13sWaitFin
	t13sDone
	t13sFailed
)

type t13sConf struct {
	certs              [][]byte
	key                []byte
	alpn               []string
	suites, groups     []int
	random, priv, salt []byte
	mlkemM             []byte
	rfc, hasEE         bool
	ee, ticket         []byte
	// session tickets (tlsticket.xo)
	ticketKeys       []t13sTicketKey
	now              int64
	ticketIV, ageAdd []byte
}

type t13Server struct {
	conf                             t13sConf
	state                            int
	err                              string
	closed, hrr, ccsSent, seen, vers bool
	resumed, tickets                 bool
	suite, group, scheme             int
	alpn, sni                        string
	rin, hs, app, out, tr            []byte
	random                           []byte
	ch1                              t13sHello
	cHS, clientFin, cAP              []byte
	rk, wk                           *t13Keys
}

func (s *t13Server) keys() {
	if s.rk == nil {
		s.rk, s.wk = &t13Keys{}, &t13Keys{}
	}
}

func (s *t13Server) take() []byte { o := s.out; s.out = nil; return o }

func (s *t13Server) send(data []byte) {
	if s.state == t13sDone {
		s.out = append(s.out, s.wk.seal(23, data, 0x0303)...)
	}
}

func (s *t13Server) close() {
	s.keys()
	if s.state != t13sFailed {
		s.out = append(s.out, s.wk.seal(21, []byte{1, 0}, 0x0303)...)
	}
}

func (s *t13Server) feedIn(data []byte, chunk int) {
	if chunk <= 0 {
		s.feed(data)
		return
	}
	for i := 0; i < len(data); i += chunk {
		s.feed(data[i:min(i+chunk, len(data))])
	}
}

func (s *t13Server) feed(data []byte) {
	s.keys()
	s.rin = append(s.rin, data...)
	for s.state != t13sFailed && !s.closed && len(s.rin) >= 5 {
		n := t13U16At(s.rin, 3)
		if !s.seen && (s.rin[0] != 21 && s.rin[0] != 22 || s.rin[1] >= 16) {
			s.err, s.state = "tls: first record does not look like a TLS handshake", t13sFailed
			return
		}
		s.seen = true
		if n > 16640 {
			s.fail(22, fmt.Sprintf("tls: oversized record received with length %d", n))
			return
		}
		if len(s.rin) < 5+n {
			return
		}
		hdr, body := s.rin[:5], s.rin[5:5+n]
		s.rin = s.rin[5+n:]
		s.record(hdr, body)
	}
}

func (s *t13Server) record(hdr, body []byte) {
	ct := int(hdr[0])
	if ct == 20 {
		if s.state == t13sDone || len(body) != 1 || body[0] != 1 {
			s.fail(10, "local error: tls: unexpected message")
		}
		return
	}
	if s.rk.on() && ct != 23 {
		s.fail(10, "local error: tls: unexpected message")
		return
	}
	ict, data := s.rk.open(hdr, body)
	switch ict {
	case -1:
		s.fail(20, "local error: tls: bad record MAC")
	case -2:
		s.fail(22, "local error: tls: record overflow")
	case -3:
		s.fail(10, "local error: tls: unexpected message")
	case 21:
		if len(data) != 2 {
			s.fail(10, "local error: tls: unexpected message")
			return
		}
		if data[1] == 0 {
			s.closed = true
			return
		}
		s.err, s.state = "remote error: tls: "+t13AlertText(int(data[1])), t13sFailed
	case 22:
		if len(data) == 0 {
			s.fail(10, "local error: tls: unexpected message")
			return
		}
		s.hs = append(s.hs, data...)
		for s.state != t13sFailed && len(s.hs) >= 4 {
			t, n := int(s.hs[0]), t13U24At(s.hs, 1)
			if n > 65536 {
				s.fail(80, fmt.Sprintf("tls: handshake message of length %d bytes exceeds maximum of %d bytes", n, 65536))
				return
			}
			if len(s.hs) < 4+n {
				return
			}
			msg := append([]byte(nil), s.hs[:4+n]...)
			s.hs = s.hs[4+n:]
			s.handshake(t, msg)
		}
	case 23:
		if s.state != t13sDone {
			s.fail(10, "local error: tls: unexpected message")
			return
		}
		s.app = append(s.app, data...)
	default:
		s.fail(10, "local error: tls: unexpected message")
	}
}

var t13sGoTypes = map[int]string{1: "*tls.clientHelloMsg", 2: "*tls.serverHelloMsg", 4: "*tls.newSessionTicketMsgTLS13",
	8: "*tls.encryptedExtensionsMsg", 11: "*tls.certificateMsgTLS13", 13: "*tls.certificateRequestMsgTLS13",
	15: "*tls.certificateVerifyMsg", 20: "*tls.finishedMsg", 24: "*tls.keyUpdateMsg"}

func (s *t13Server) handshake(t int, msg []byte) {
	if t13sGoTypes[t] == "" {
		s.fail(10, "local error: tls: unexpected message")
		return
	}
	switch {
	case s.state == t13sWaitCH && t == 1:
		s.clientHello(msg)
	case s.state == t13sWaitCH2 && t == 1:
		s.clientHello2(msg)
	case s.state == t13sWaitFin && t == 20:
		s.finished(msg)
	case s.state == t13sDone && t == 24:
		s.keyUpdate(msg)
	default:
		want := "*tls.clientHelloMsg"
		if s.state == t13sWaitFin {
			want = "*tls.finishedMsg"
		} else if s.state == t13sDone {
			want = "*tls.keyUpdateMsg"
		}
		s.fail(10, fmt.Sprintf("tls: received unexpected handshake message of type %s when waiting for %s", t13sGoTypes[t], want))
	}
}

type t13sHello struct {
	ok                             bool
	vers                           int
	random, sid, comp, reneg, kept []byte
	suites, groups, algs, versions []int
	shares                         []int
	shareKeys                      [][]byte
	name                           string
	alpn                           []string
	early, quic                    bool
	raw, pskModes                  []byte
	ids, binders                   [][]byte
	bindersLen                     int
}

func t13sParseHello(m []byte) (h t13sHello) {
	h.raw = m
	p := 4
	if len(m) < p+35 {
		return h
	}
	h.vers = t13U16At(m, p)
	h.random = m[p+2 : p+34]
	p += 34
	sn := int(m[p])
	if sn > 32 || len(m) < p+1+sn+2 {
		return h
	}
	h.sid = m[p+1 : p+1+sn]
	p += 1 + sn
	cn := t13U16At(m, p)
	if cn%2 != 0 || len(m) < p+2+cn+1 {
		return h
	}
	for i := p + 2; i < p+2+cn; i += 2 {
		h.suites = append(h.suites, t13U16At(m, i))
	}
	suites := m[p+2 : p+2+cn]
	p += 2 + cn
	mn := int(m[p])
	if len(m) < p+1+mn {
		return h
	}
	h.comp = m[p+1 : p+1+mn]
	p += 1 + mn
	kept := t13U16(nil, h.vers)
	kept = append(kept, h.random...)
	kept = append(append(kept, byte(len(h.sid))), h.sid...)
	kept = append(kept, t13Vec16(suites)...)
	kept = append(append(kept, byte(len(h.comp))), h.comp...)
	if p == len(m) {
		h.kept, h.ok = kept, true
		return h
	}
	if len(m) < p+2 || p+2+t13U16At(m, p) != len(m) {
		return h
	}
	p += 2
	seen := map[int]bool{}
	for p < len(m) {
		if len(m) < p+4 {
			return h
		}
		t, n := t13U16At(m, p), t13U16At(m, p+2)
		if len(m) < p+4+n || seen[t] {
			return h
		}
		seen[t] = true
		d := m[p+4 : p+4+n]
		p += 4 + n
		if !h.extension(t, d, p == len(m)) {
			return h
		}
		switch t {
		case 0, 5, 10, 11, 13, 16, 18, 23, 35, 43, 44, 45, 50, 0xff01:
			kept = t13Ext(kept, t, d)
		}
	}
	h.kept, h.ok = kept, true
	return h
}

func (h *t13sHello) extension(t int, d []byte, last bool) bool {
	n := len(d)
	switch t {
	case 0:
		if n < 2 || t13U16At(d, 0) != n-2 || n == 2 {
			return false
		}
		for q := 2; q < n; {
			if n < q+3 || n < q+3+t13U16At(d, q+1) || t13U16At(d, q+1) == 0 {
				return false
			}
			k := t13U16At(d, q+1)
			if d[q] == 0 {
				if h.name != "" {
					return false
				}
				h.name = strings.ToValidUTF8(string(d[q+3:q+3+k]), "�")
				if strings.HasSuffix(h.name, ".") {
					return false
				}
			}
			q += 3 + k
		}
	case 10, 13, 50:
		if n < 2 || t13U16At(d, 0) != n-2 || n == 2 || n%2 != 0 {
			return false
		}
		for q := 2; q < n; q += 2 {
			if t == 10 {
				h.groups = append(h.groups, t13U16At(d, q))
			} else if t == 13 {
				h.algs = append(h.algs, t13U16At(d, q))
			}
		}
	case 16:
		if n < 2 || t13U16At(d, 0) != n-2 || n == 2 {
			return false
		}
		for q := 2; q < n; {
			k := int(d[q])
			if k == 0 || n < q+1+k {
				return false
			}
			h.alpn = append(h.alpn, strings.ToValidUTF8(string(d[q+1:q+1+k]), "�"))
			q += 1 + k
		}
	case 43:
		if n < 1 || int(d[0]) != n-1 || n == 1 || n%2 != 1 {
			return false
		}
		for q := 1; q < n; q += 2 {
			h.versions = append(h.versions, t13U16At(d, q))
		}
	case 51:
		if n < 2 || t13U16At(d, 0) != n-2 {
			return false
		}
		for q := 2; q < n; {
			if n < q+4 || n < q+4+t13U16At(d, q+2) || t13U16At(d, q+2) == 0 {
				return false
			}
			k := t13U16At(d, q+2)
			h.shares = append(h.shares, t13U16At(d, q))
			h.shareKeys = append(h.shareKeys, d[q+4:q+4+k])
			q += 4 + k
		}
	case 44:
		return n > 2 && t13U16At(d, 0) == n-2
	case 45:
		if n < 1 || int(d[0]) != n-1 {
			return false
		}
		h.pskModes = d[1:n]
	case 0xff01:
		if n < 1 || int(d[0]) != n-1 {
			return false
		}
		h.reneg = d[1:n]
	case 42:
		h.early = true
	case 57:
		h.quic = true
	case 41:
		if !last || n < 2 || n < 2+t13U16At(d, 0) || t13U16At(d, 0) == 0 {
			return false
		}
		il := t13U16At(d, 0)
		q := 2
		for q < 2+il {
			if 2+il < q+2 || 2+il < q+2+t13U16At(d, q)+4 || t13U16At(d, q) == 0 {
				return false
			}
			h.ids = append(h.ids, d[q+2:q+2+t13U16At(d, q)])
			q += 2 + t13U16At(d, q) + 4
		}
		if n < q+2 || q+2+t13U16At(d, q) != n || t13U16At(d, q) == 0 {
			return false
		}
		h.bindersLen = n - q
		for q += 2; q < n; {
			k := int(d[q])
			if k == 0 || n < q+1+k {
				return false
			}
			h.binders = append(h.binders, d[q+1:q+1+k])
			q += 1 + k
		}
	}
	return true
}

func t13sHexList(xs []int) string {
	var ps []string
	for _, x := range xs {
		ps = append(ps, fmt.Sprintf("%x", x))
	}
	return "[" + strings.Join(ps, " ") + "]"
}

func (s *t13Server) clientHello(msg []byte) {
	h := t13sParseHello(msg)
	if !h.ok {
		s.fail(50, "local error: tls: error decoding message")
		return
	}
	versions := h.versions
	if len(versions) == 0 {
		for _, x := range []int{0x0304, 0x0303, 0x0302, 0x0301} {
			if x <= h.vers {
				versions = append(versions, x)
			}
		}
	}
	if !t13Contains(versions, 0x0304) {
		s.fail(70, "tls: client offered only unsupported versions: "+t13sHexList(versions))
		return
	}
	s.vers = true
	if len(h.comp) != 1 || h.comp[0] != 0 {
		s.fail(47, "tls: TLS 1.3 client supports illegal compression methods")
		return
	}
	s.random = s.conf.random
	if len(s.random) == 0 {
		s.random = make([]byte, 32)
		rand.Read(s.random)
	}
	if len(h.reneg) != 0 {
		s.fail(40, "tls: initial handshake had non-empty renegotiation extension")
		return
	}
	if h.early {
		s.fail(110, "tls: client sent unexpected early data")
		return
	}
	order := []int{0x1303, 0x1301, 0x1302}
	for _, id := range h.suites {
		if id >= 0x1301 && id <= 0x1303 {
			if id != 0x1303 {
				order = []int{0x1301, 0x1302, 0x1303}
			}
			break
		}
	}
	for _, id := range order {
		if s.suite == 0 && t13Contains(h.suites, id) && t13Contains(s.conf.suites, id) {
			s.suite = id
		}
	}
	if s.suite == 0 {
		s.fail(40, "tls: no cipher suite supported by both client and server; client offered: "+t13sHexList(h.suites))
		return
	}
	var shared []int
	for _, g := range s.conf.groups {
		if t13Contains(h.groups, g) && (g == 23 || g == 24 || g == 25 || g == 29 || t13IsHybrid(g)) {
			shared = append(shared, g)
		}
	}
	if len(shared) == 0 {
		s.fail(40, "tls: no key exchanges supported by both client and server")
		return
	}
	// Go 1.26's two stable sorts: a hybrid first, then a group with a share
	sort.SliceStable(shared, func(i, j int) bool {
		return t13Contains(h.shares, shared[i]) && !t13Contains(h.shares, shared[j])
	})
	sort.SliceStable(shared, func(i, j int) bool {
		return t13IsHybrid(shared[i]) && !t13IsHybrid(shared[j])
	})
	group := shared[0]
	s.group, s.ch1, s.tr = group, h, append([]byte(nil), msg...)
	if !t13Contains(h.shares, group) {
		s.retry(msg)
		return
	}
	s.respond(h)
}

func (s *t13Server) retry(msg []byte) {
	if len(s.hs) != 0 {
		s.fail(10, "tls: handshake buffer not empty before HelloRetryRequest")
		return
	}
	hf, _ := t13HashOf(s.suite)
	hrrRandom := sha256.Sum256([]byte("HelloRetryRequest"))
	exts := t13Ext(nil, 43, []byte{3, 4})
	exts = t13Ext(exts, 51, t13U16(nil, s.group))
	body := append(t13U16(nil, 0x0303), hrrRandom[:]...)
	body = append(append(body, byte(len(s.ch1.sid))), s.ch1.sid...)
	body = append(t13U16(body, s.suite), 0)
	hrr := t13Message(2, append(body, t13Vec16(exts)...))
	s.tr = append(t13Message(254, t13Sum(hf, msg)), hrr...)
	s.out = append(s.out, s.wk.seal(22, hrr, 0x0303)...)
	s.dummyCCS()
	s.hrr, s.state = true, t13sWaitCH2
}

func (s *t13Server) clientHello2(msg []byte) {
	h := t13sParseHello(msg)
	switch {
	case !h.ok:
		s.fail(50, "local error: tls: error decoding message")
	case len(h.shares) != 1:
		s.fail(47, "tls: client didn't send one key share in second ClientHello")
	case h.shares[0] != s.group:
		s.fail(47, "tls: client sent unexpected key share in second ClientHello")
	case h.early:
		s.fail(47, "tls: client indicated early data in second ClientHello")
	case !bytes.Equal(h.kept, s.ch1.kept):
		s.fail(47, "tls: client illegally modified second ClientHello")
	default:
		s.tr = append(s.tr, msg...)
		s.respond(h)
	}
}

func (s *t13Server) respond(h t13sHello) {
	var peer []byte
	for i, g := range h.shares {
		if g == s.group && peer == nil {
			peer = h.shareKeys[i]
		}
	}
	ecdhGroup := s.group
	if t13IsHybrid(s.group) {
		ecdhGroup = t13HybridECDH(s.group)
	}
	priv := s.conf.priv
	if len(priv) == 0 {
		k, _ := t13Curve(ecdhGroup).GenerateKey(rand.Reader)
		priv = k.Bytes()
	}
	var share, shared []byte
	ok := false
	if t13IsHybrid(s.group) {
		m := s.conf.mlkemM
		if len(m) != 32 {
			m = make([]byte, 32)
			rand.Read(m)
		}
		shared, share, ok = t13sHybridServer(s.group, peer, priv, m)
	} else {
		share = t13PublicKey(s.group, priv)
		shared, ok = t13Shared(s.group, priv, peer)
	}
	if !ok {
		s.fail(47, "tls: invalid client key share")
		return
	}
	alpn, err := t13sNegotiateALPN(s.conf.alpn, h.alpn)
	if err != "" {
		s.fail(120, err)
		return
	}
	s.alpn = alpn
	if h.quic {
		s.fail(110, "tls: client sent an unexpected quic_transport_parameters extension")
		return
	}
	s.sni = h.name
	hf, l := t13HashOf(s.suite)
	var psk []byte
	chosen := -1
	modeDHE := bytes.IndexByte(h.pskModes, 1) >= 0
	if len(s.conf.ticketKeys) > 0 && modeDHE {
		if len(h.ids) != len(h.binders) {
			s.fail(47, "tls: invalid or missing PSK binders")
			return
		}
		before := s.tr[:len(s.tr)-len(h.raw)]
		partial := append(append([]byte(nil), before...), h.raw[:len(h.raw)-h.bindersLen]...)
		for i, id := range h.ids {
			if chosen >= 0 || i >= 5 {
				break
			}
			st, ok := s.session(id, hf)
			if !ok {
				continue
			}
			early, _ := hkdf.Extract(hf, st.secret, nil)
			binderKey := t13ExpandLabel(hf, early, "res binder", t13Sum(hf, nil), l)
			if !hmac.Equal(h.binders[i], t13VerifyData(hf, l, binderKey, partial)) {
				s.fail(51, "tls: invalid PSK binder")
				return
			}
			psk, chosen = st.secret, i
		}
	}
	s.resumed = chosen >= 0
	s.tickets = len(s.conf.ticketKeys) > 0 && modeDHE
	if s.resumed {
		s.finishHello(h, share, shared, psk, chosen)
		return
	}
	if len(h.algs) == 0 {
		s.fail(109, "local error: tls: missing extension")
		return
	}
	leaf, perr := x509.ParseCertificate(s.conf.certs[0])
	if perr != nil {
		s.fail(80, "tls: failed to parse certificate: "+perr.Error())
		return
	}
	mine := t13sSchemesFor(leaf.PublicKey)
	for _, a := range h.algs {
		if s.scheme == 0 && t13Contains(mine, a) {
			s.scheme = a
		}
	}
	if s.scheme == 0 {
		s.fail(40, "tls: peer doesn't support any of the certificate's signature algorithms")
		return
	}
	s.finishHello(h, share, shared, nil, -1)
}

func (s *t13Server) finishHello(h t13sHello, share, shared, psk []byte, chosen int) {
	ks := t13Ext(nil, 51, append(t13U16(nil, s.group), t13Vec16(share)...))
	sv := t13Ext(nil, 43, []byte{3, 4})
	exts := append(sv, ks...)
	if s.conf.rfc {
		exts = append(ks, sv...)
	}
	if chosen >= 0 {
		exts = t13Ext(exts, 41, t13U16(nil, chosen))
	}
	body := append(t13U16(nil, 0x0303), s.random...)
	body = append(append(body, byte(len(h.sid))), h.sid...)
	body = append(t13U16(body, s.suite), 0)
	sh := t13Message(2, append(body, t13Vec16(exts)...))
	s.tr = append(s.tr, sh...)
	s.out = append(s.out, s.wk.seal(22, sh, 0x0303)...)
	s.dummyCCS()
	hf, l := t13HashOf(s.suite)
	if psk == nil {
		psk = make([]byte, l)
	}
	early, _ := hkdf.Extract(hf, psk, nil)
	derived := t13ExpandLabel(hf, early, "derived", t13Sum(hf, nil), l)
	hsSecret, _ := hkdf.Extract(hf, shared, derived)
	th := t13Sum(hf, s.tr)
	s.cHS = t13ExpandLabel(hf, hsSecret, "c hs traffic", th, l)
	sHS := t13ExpandLabel(hf, hsSecret, "s hs traffic", th, l)
	s.wk, s.rk = t13NewKeys(s.suite, sHS), t13NewKeys(s.suite, s.cHS)
	eeExts := s.conf.ee
	if !s.conf.hasEE {
		eeExts = nil
		if s.alpn != "" {
			eeExts = t13Ext(eeExts, 16, t13Vec16(append([]byte{byte(len(s.alpn))}, s.alpn...)))
		}
		if s.sni != "" && chosen < 0 {
			eeExts = t13Ext(eeExts, 0, nil)
		}
	}
	ee := t13Message(8, t13Vec16(eeExts))
	s.tr = append(s.tr, ee...)
	flight := append([]byte(nil), ee...)
	if chosen < 0 {
		cc := s.certFlight(hf)
		if s.state == t13sFailed {
			return
		}
		flight = append(flight, cc...)
	}
	fin := t13Message(20, t13VerifyData(hf, l, sHS, s.tr))
	s.tr = append(s.tr, fin...)
	s.out = append(s.out, s.wk.seal(22, append(flight, fin...), 0x0303)...)
	derived = t13ExpandLabel(hf, hsSecret, "derived", t13Sum(hf, nil), l)
	master, _ := hkdf.Extract(hf, make([]byte, l), derived)
	th = t13Sum(hf, s.tr)
	s.cAP = t13ExpandLabel(hf, master, "c ap traffic", th, l)
	s.wk = t13NewKeys(s.suite, t13ExpandLabel(hf, master, "s ap traffic", th, l))
	s.clientFin = t13VerifyData(hf, l, s.cHS, s.tr)
	if s.tickets {
		cf := t13Message(20, s.clientFin)
		res := t13ExpandLabel(hf, master, "res master", t13Sum(hf, append(append([]byte(nil), s.tr...), cf...)), l)
		s.out = append(s.out, s.wk.seal(22, s.ticketMsg(t13ExpandLabel(hf, res, "resumption", nil, l)), 0x0303)...)
	}
	s.state = t13sWaitFin
}

func (s *t13Server) certFlight(hf func() hash.Hash) []byte {
	var entries []byte
	for _, c := range s.conf.certs {
		entries = append(entries, byte(len(c)>>16), byte(len(c)>>8), byte(len(c)))
		entries = append(append(entries, c...), 0, 0)
	}
	cm := t13Message(11, append([]byte{0, byte(len(entries) >> 16), byte(len(entries) >> 8), byte(len(entries))}, entries...))
	s.tr = append(s.tr, cm...)
	content := append(bytes.Repeat([]byte(" "), 64), "TLS 1.3, server CertificateVerify\x00"...)
	content = append(content, t13Sum(hf, s.tr)...)
	sig := t13sSign(s.scheme, s.conf.key, content, s.conf.salt)
	if len(sig) == 0 {
		s.fail(80, "tls: failed to sign handshake: the private key does not match the certificate")
		return nil
	}
	cv := t13Message(15, append(t13U16(nil, s.scheme), t13Vec16(sig)...))
	s.tr = append(s.tr, cv...)
	return append(cm, cv...)
}

// session is tls13srv.xo's session: Go's checkForResumption checks.
func (s *t13Server) session(id []byte, hf func() hash.Hash) (t13sState, bool) {
	plain, ok := t13sTicketOpen(id, s.conf.ticketKeys)
	if !ok {
		return t13sState{}, false
	}
	st, ok := t13sParseState(plain)
	if !ok || st.version != 0x0304 || s.conf.now-st.created > 604800 {
		return t13sState{}, false
	}
	if st.suite < 0x1301 || st.suite > 0x1303 {
		return t13sState{}, false
	}
	sh, _ := t13HashOf(st.suite)
	if sh().Size() != hf().Size() {
		return t13sState{}, false
	}
	return st, true
}

func (s *t13Server) ticketMsg(psk []byte) []byte {
	r := s.conf.ageAdd
	if len(r) != 4 {
		r = make([]byte, 4)
		rand.Read(r)
	}
	iv := s.conf.ticketIV
	if len(iv) != 16 {
		iv = make([]byte, 16)
		rand.Read(iv)
	}
	label := t13sTicketSeal(s.conf.ticketKeys[0], t13sStateBytes(0x0304, s.suite, s.conf.now, psk, false, 0), iv)
	body := []byte{0, 9, 0x3a, 0x80, r[3], r[2], r[1], r[0], 0}
	body = append(body, t13Vec16(label)...)
	body = append(body, 0, 0)
	return t13Message(4, body)
}

func (s *t13Server) dummyCCS() {
	if !s.ccsSent && !s.conf.rfc {
		s.ccsSent = true
		s.out = append(s.out, 0x14, 3, 3, 0, 1, 1)
	}
}

func (s *t13Server) finished(msg []byte) {
	if !hmac.Equal(msg[4:], s.clientFin) {
		s.fail(51, "tls: invalid client finished hash")
		return
	}
	if len(s.hs) != 0 {
		s.fail(10, "tls: handshake buffer not empty before setting read traffic secret")
		return
	}
	s.rk = t13NewKeys(s.suite, s.cAP)
	if len(s.conf.ticket) > 0 {
		s.out = append(s.out, s.wk.seal(22, s.conf.ticket, 0x0303)...)
	}
	s.cHS, s.cAP, s.clientFin, s.tr = nil, nil, nil, nil
	s.state = t13sDone
}

func (s *t13Server) keyUpdate(msg []byte) {
	if len(msg) != 5 || msg[4] > 1 {
		s.fail(50, "local error: tls: error decoding message")
		return
	}
	if len(s.hs) != 0 {
		s.fail(10, "tls: handshake buffer not empty before setting read traffic secret")
		return
	}
	s.rk = s.rk.update()
	if msg[4] == 1 {
		s.out = append(s.out, s.wk.seal(22, t13Message(24, []byte{0}), 0x0303)...)
		s.wk = s.wk.update()
	}
}

func (s *t13Server) fail(alert int, msg string) {
	if s.state != t13sFailed {
		s.err, s.state = msg, t13sFailed
		vers := 0x0301
		if s.vers {
			vers = 0x0303
		}
		s.out = append(s.out, s.wk.seal(21, []byte{2, byte(alert)}, vers)...)
	}
}

func t13sNegotiateALPN(server, client []string) (string, string) {
	if len(server) == 0 || len(client) == 0 {
		return "", ""
	}
	fallback := false
	for _, sp := range server {
		for _, cp := range client {
			if sp == cp {
				return sp, ""
			}
			if sp == "h2" && cp == "http/1.1" {
				fallback = true
			}
		}
	}
	if fallback {
		return "", ""
	}
	return "", fmt.Sprintf("tls: client requested unsupported application protocols (%q)", client)
}

func t13sSchemesFor(pub any) []int {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		if k.Curve == elliptic.P256() {
			return []int{0x0403}
		}
		if k.Curve == elliptic.P384() {
			return []int{0x0503}
		}
		if k.Curve == elliptic.P521() {
			return []int{0x0603}
		}
	case ed25519.PublicKey:
		return []int{0x0807}
	case *rsa.PublicKey:
		var out []int
		for _, c := range []struct{ scheme, h int }{{0x0804, 32}, {0x0805, 48}, {0x0806, 64}} {
			if k.Size() >= 2*c.h+2 {
				out = append(out, c.scheme)
			}
		}
		return out
	}
	return nil
}

func t13sSign(scheme int, key, content, salt []byte) []byte {
	switch scheme {
	case 0x0807:
		if len(key) != 32 {
			return nil
		}
		return ed25519.Sign(ed25519.NewKeyFromSeed(key), content)
	case 0x0403, 0x0503, 0x0603:
		curve, h := elliptic.P256(), crypto.SHA256
		if scheme == 0x0503 {
			curve, h = elliptic.P384(), crypto.SHA384
		} else if scheme == 0x0603 {
			curve, h = elliptic.P521(), crypto.SHA512
		}
		k, err := ecdsa.ParseRawPrivateKey(curve, key)
		if err != nil {
			return nil
		}
		hh := h.New()
		hh.Write(content)
		sig, _ := ecdsa.SignASN1(rand.Reader, k, hh.Sum(nil))
		return sig
	}
	h := crypto.SHA256
	if scheme == 0x0805 {
		h = crypto.SHA384
	} else if scheme == 0x0806 {
		h = crypto.SHA512
	}
	k, err := t13sRSAKey(key)
	if err != nil {
		return nil
	}
	hh := h.New()
	hh.Write(content)
	var r io.Reader = rand.Reader
	if len(salt) > 0 {
		r = bytes.NewReader(salt)
	}
	sig, _ := rsa.SignPSS(r, k, h, hh.Sum(nil), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	return sig
}

func t13sRSAKey(der []byte) (*rsa.PrivateKey, error) {
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA key")
	}
	return rk, nil
}

// t13sPair is tls13srv.xo's pair with Go's crypto/tls client and server
// over an in-memory connection.
func t13sPair(certDER, key []byte, args []string) string {
	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		return "parse\n"
	}
	var signer crypto.Signer
	switch pub := leaf.PublicKey.(type) {
	case *rsa.PublicKey:
		signer, err = t13sRSAKey(key)
	case *ecdsa.PublicKey:
		signer, err = ecdsa.ParseRawPrivateKey(pub.Curve, key)
	case ed25519.PublicKey:
		signer = ed25519.NewKeyFromSeed(key)
	}
	if err != nil || signer == nil {
		return "key\n"
	}
	curves := func(s string) (out []tls.CurveID) {
		for _, g := range t13sInts(s, 10) {
			out = append(out, tls.CurveID(g))
		}
		return out
	}
	protos := func(s string) []string {
		if s == "-" {
			return nil
		}
		return strings.Split(s, ",")
	}
	host := args[4]
	if host == "-" {
		host = ""
	}
	ccf := &tls.Config{InsecureSkipVerify: true, ServerName: host, NextProtos: protos(args[1]), CurvePreferences: curves(args[0]),
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	scf := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{certDER}, PrivateKey: signer}}, NextProtos: protos(args[3]),
		CurvePreferences: curves(args[2]), MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, SessionTicketsDisabled: true}
	cc, sc := t13sPipe()
	client, server := tls.Client(cc, ccf), tls.Server(sc, scf)
	type result struct {
		err    error
		got    string
		closed bool
	}
	done := make(chan result, 1)
	go func() {
		var r result
		defer func() { server.Close(); done <- r }()
		if r.err = server.Handshake(); r.err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		r.got = string(buf[:n])
		server.Write([]byte("pong"))
		_, rerr := server.Read(buf)
		r.closed = rerr == io.EOF
	}()
	cerr := client.Handshake()
	errText := func(e error) string {
		if e == nil {
			return "-"
		}
		return e.Error()
	}
	var out string
	if cerr != nil {
		client.Close()
		r := <-done
		return fmt.Sprintf("client %s\nserver %s\n", errText(cerr), errText(r.err))
	}
	client.Write([]byte("ping"))
	buf := make([]byte, 64)
	n, _ := client.Read(buf)
	client.Close()
	r := <-done
	out = fmt.Sprintf("client %s\nserver %s\n", errText(cerr), errText(r.err))
	if r.err == nil {
		st, cst := server.ConnectionState(), client.ConnectionState()
		out += fmt.Sprintf("params %s %d %s %s %s\n", tls.CipherSuiteName(st.CipherSuite), int(st.CurveID), t13sDash(st.NegotiatedProtocol),
			t13sDash(cst.NegotiatedProtocol), t13sDash(st.ServerName))
		out += fmt.Sprintf("server got %s\nclient got %s\nclosed %v\n", r.got, string(buf[:n]), r.closed)
	}
	return out
}

// t13sPipe is a connected pair like net.Pipe whose writes never block
// (net.Pipe's do until read, and both sides of a HelloRetryRequest
// write at once: the server's dummy ChangeCipherSpec, the client's second
// ClientHello).
func t13sPipe() (net.Conn, net.Conn) {
	a, b := &t13sBuf{}, &t13sBuf{}
	a.cond, b.cond = sync.NewCond(&a.mu), sync.NewCond(&b.mu)
	x, y := net.Pipe()
	return &t13sConn{Conn: x, r: a, w: b}, &t13sConn{Conn: y, r: b, w: a}
}

type t13sBuf struct {
	mu     sync.Mutex
	cond   *sync.Cond
	data   []byte
	closed bool
}

type t13sConn struct {
	net.Conn
	r, w *t13sBuf
}

func (c *t13sConn) Read(p []byte) (int, error) {
	c.r.mu.Lock()
	defer c.r.mu.Unlock()
	for len(c.r.data) == 0 && !c.r.closed {
		c.r.cond.Wait()
	}
	if len(c.r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.r.data)
	c.r.data = c.r.data[n:]
	return n, nil
}

func (c *t13sConn) Write(p []byte) (int, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	if c.w.closed {
		return 0, io.ErrClosedPipe
	}
	c.w.data = append(c.w.data, p...)
	c.w.cond.Broadcast()
	return len(p), nil
}

func (c *t13sConn) Close() error {
	for _, b := range []*t13sBuf{c.r, c.w} {
		b.mu.Lock()
		b.closed = true
		b.cond.Broadcast()
		b.mu.Unlock()
	}
	return c.Conn.Close()
}

// The session tickets of tlsticket.xo, written out with Go's crypto: Go's
// ticketKeyFromBytes, SessionState.Bytes and ParseSessionState for a
// server's session, encryptTicket and decryptTicket.

type t13sTicketKey struct {
	aes, hmac []byte
	created   int64
}

func t13sKeyFrom(seed []byte, created int64) t13sTicketKey {
	h := sha512.Sum512(seed)
	return t13sTicketKey{aes: h[16:32], hmac: h[32:48], created: created}
}

type t13sState struct {
	version, suite, curve int
	created               int64
	secret                []byte
	ems                   bool
}

func t13sStateBytes(version, suite int, created int64, secret []byte, ems bool, curve int) []byte {
	b := t13U16(nil, version)
	b = append(b, 1)
	b = t13U16(b, suite)
	b = binary.BigEndian.AppendUint64(b, uint64(created))
	b = append(append(b, byte(len(secret))), secret...)
	b = append(b, 0, 0, 0)
	if ems {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	b = append(b, 0, 0, 0, 0, 0, 0, 0)
	if version < 0x0304 {
		b = t13U16(b, curve)
	}
	return b
}

func t13sParseState(b []byte) (st t13sState, ok bool) {
	n := len(b)
	if n < 14 {
		return st, false
	}
	st.version, st.suite = t13U16At(b, 0), t13U16At(b, 3)
	typ := b[2]
	st.created = int64(binary.BigEndian.Uint64(b[5:13]))
	p := 13
	sl := int(b[p])
	if sl == 0 || n < p+1+sl+3 {
		return st, false
	}
	st.secret = b[p+1 : p+1+sl]
	p += 1 + sl
	xl := t13U24At(b, p)
	if n < p+3+xl {
		return st, false
	}
	for q := p + 3; q < p+3+xl; q += 3 + t13U24At(b, q) {
		if p+3+xl < q+3 || p+3+xl < q+3+t13U24At(b, q) {
			return st, false
		}
	}
	p += 3 + xl
	if n < p+2+3 {
		return st, false
	}
	ems, early := b[p], b[p+1]
	p += 2
	if (typ != 1 && typ != 2) || ems > 1 || early > 1 {
		return st, false
	}
	st.ems = ems == 1
	if t13U24At(b, p) != 0 || n < p+6 || t13U24At(b, p+3) != 0 {
		return st, false
	}
	p += 6
	if early == 1 {
		if n < p+1+int(b[p]) {
			return st, false
		}
		p += 1 + int(b[p])
	}
	if st.version >= 0x0304 {
		if typ == 2 && n < p+12 {
			return st, false
		}
	} else {
		if n < p+2 {
			return st, false
		}
		st.curve = t13U16At(b, p)
	}
	return st, true
}

func t13sTicketSeal(k t13sTicketKey, state, iv []byte) []byte {
	out := append([]byte(nil), iv...)
	ct := make([]byte, len(state))
	blk, _ := aes.NewCipher(k.aes)
	cipher.NewCTR(blk, iv).XORKeyStream(ct, state)
	out = append(out, ct...)
	m := hmac.New(sha256.New, k.hmac)
	m.Write(out)
	return m.Sum(out)
}

func t13sTicketOpen(t []byte, keys []t13sTicketKey) ([]byte, bool) {
	if len(t) < 16+32 {
		return nil, false
	}
	body, mac := t[:len(t)-32], t[len(t)-32:]
	for _, k := range keys {
		m := hmac.New(sha256.New, k.hmac)
		m.Write(body)
		if hmac.Equal(m.Sum(nil), mac) {
			blk, _ := aes.NewCipher(k.aes)
			pt := make([]byte, len(body)-16)
			cipher.NewCTR(blk, t[:16]).XORKeyStream(pt, t[16:len(t)-32])
			return pt, true
		}
	}
	return nil, false
}

// t13sHybridServer is crypto/tls's hybridKeyExchange.serverSharedSecret
// for the hybrid group with the ECDH key priv and the ML-KEM message m:
// the secret and the share, ML-KEM's part first for X25519MLKEM768 and
// the ECDH part first for SecP256r1MLKEM768 and SecP384r1MLKEM1024.
func t13sHybridServer(group int, peer, priv, m []byte) ([]byte, []byte, bool) {
	eg := t13HybridECDH(group)
	es := len(t13PublicKey(eg, priv))
	en := mlkem.EncapsulationKeySize768
	if group == t13HybridP384 {
		en = mlkem.EncapsulationKeySize1024
	}
	if len(peer) != en+es {
		return nil, nil, false
	}
	eShare, ekb := peer[:es], peer[es:]
	if group == t13Hybrid {
		ekb, eShare = peer[:en], peer[en:]
	}
	xs, ok := t13Shared(eg, priv, eShare)
	if !ok {
		return nil, nil, false
	}
	var k, ct []byte
	var err error
	if group == t13HybridP384 {
		ek, e := mlkem.NewEncapsulationKey1024(ekb)
		if e != nil {
			return nil, nil, false
		}
		k, ct, err = mlkemtest.Encapsulate1024(ek, m)
	} else {
		ek, e := mlkem.NewEncapsulationKey768(ekb)
		if e != nil {
			return nil, nil, false
		}
		k, ct, err = mlkemtest.Encapsulate768(ek, m)
	}
	if err != nil {
		return nil, nil, false
	}
	pub := t13PublicKey(eg, priv)
	if group == t13Hybrid {
		return append(k, xs...), append(ct, pub...), true
	}
	return append(xs, k...), append(pub, ct...), true
}
