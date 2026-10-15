package rt

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

// Internal_crypto_op is `internal.crypto_op(op, a, b, c, d)`: Go's crypto
// as the reference for the native TLS primitives (work item B31). The one
// dispatcher: each area of std/internal.xo's list is one case calling
// its function here.
func Internal_crypto_op(_ *Task, op string, a, b, c, d Bytes) Bytes {
	area, what, _ := strings.Cut(op, ".")
	var r Bytes
	ok := false
	switch area {
	case "hash":
		r, ok = cryptoHash(what, a, b)
	case "hmac":
		r, ok = cryptoHMAC(what, a, b)
	case "hkdf":
		r, ok = cryptoHKDF(what, a, b, c, d)
	case "p256":
		r, ok = cryptoEC(elliptic.P256(), ecdh.P256(), what, a, b, c)
	case "p384":
		r, ok = cryptoEC(elliptic.P384(), ecdh.P384(), what, a, b, c)
	case "rsa":
		r, ok = cryptoRSA(what, a, b, c, d)
	case "x509":
		r, ok = cryptoX509(what, a, b)
	case "aes":
		r, ok = cryptoAES(what, a, b, c, d)
	case "chacha":
		r, ok = cryptoChaCha(what, a, b, c, d)
	case "x25519":
		r, ok = cryptoX25519(what, a, b)
	case "chain":
		r, ok = cryptoChain(what, a, b, c, d)
	case "tls13":
		r, ok = cryptoTLS13(what, a, b, c, d)
	}
	if !ok {
		panic("internal.crypto_op: unknown op " + op)
	}
	return r
}

// cryptoHashOf is the hash a crypto_op name ends in ("...256", "...384", or "...512").
func cryptoHashOf(what string) (func() hash.Hash, string) {
	f := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(what, "256"), "384"), "512")
	if strings.HasSuffix(what, "384") {
		return sha512.New384, f
	}
	if strings.HasSuffix(what, "512") {
		return sha512.New, f
	}
	return sha256.New, f
}

func cryptoNum(x Bytes) int { n, _ := strconv.Atoi(string(x)); return n }

func cryptoSum(x hash.Hash) Bytes { return Bytes(x.Sum(nil)) }

// cryptoHash is sha256.xo's op.
func cryptoHash(what string, a, b Bytes) (Bytes, bool) {
	h, f := cryptoHashOf(what)
	switch f {
	case "sha":
		x := h()
		x.Write([]byte(a))
		return cryptoSum(x), true
	case "split":
		x := h()
		x.Write([]byte(a))
		s1 := cryptoSum(x)
		x.Write([]byte(b))
		return s1 + cryptoSum(x) + s1, true
	}
	return "", false
}

// cryptoHMAC is hmac.xo's op.
func cryptoHMAC(what string, a, b Bytes) (Bytes, bool) {
	h, f := cryptoHashOf(what)
	if f != "sha" {
		return "", false
	}
	m := hmac.New(h, []byte(a))
	m.Write([]byte(b))
	return cryptoSum(m), true
}

// cryptoHKDF is hkdf.xo's op.
func cryptoHKDF(what string, a, b, c, d Bytes) (Bytes, bool) {
	h, f := cryptoHashOf(what)
	var k []byte
	var err error
	switch f {
	case "extract":
		k, err = hkdf.Extract(h, []byte(b), []byte(a))
	case "expand":
		k, err = hkdf.Expand(h, []byte(a), string(b), cryptoNum(c))
	case "label":
		full := "tls13 " + string(b)
		n := cryptoNum(d)
		info := []byte{byte(n >> 8), byte(n), byte(len(full))}
		info = append(info, full...)
		info = append(info, byte(len(c)))
		info = append(info, c...)
		k, err = hkdf.Expand(h, []byte(a), string(info), n)
	default:
		return "", false
	}
	if err != nil {
		panic(err)
	}
	return Bytes(k), true
}

func cryptoFlag(x bool) Bytes {
	if x {
		return "\x01"
	}
	return "\x00"
}

// cryptoEC is nistec.xo's op.
func cryptoEC(curve elliptic.Curve, ec ecdh.Curve, what string, a, b, c Bytes) (Bytes, bool) {
	switch what {
	case "public":
		k, err := ec.NewPrivateKey([]byte(a))
		if err != nil {
			return "", true
		}
		return Bytes(k.PublicKey().Bytes()), true
	case "ecdh":
		k, err := ec.NewPrivateKey([]byte(a))
		if err != nil {
			return "", true
		}
		p, err := ec.NewPublicKey([]byte(b))
		if err != nil {
			return "", true
		}
		s, err := k.ECDH(p)
		if err != nil {
			return "", true
		}
		return Bytes(s), true
	case "generate":
		k, err := ec.GenerateKey(rand.Reader)
		if err != nil {
			panic(err)
		}
		return Bytes(k.Bytes()), true
	case "sign":
		k, err := ecdsa.ParseRawPrivateKey(curve, []byte(a))
		if err != nil {
			return "", true
		}
		sig, err := ecdsa.SignASN1(rand.Reader, k, []byte(b))
		if err != nil {
			panic(err)
		}
		return Bytes(sig), true
	case "verify":
		k, err := ecdsa.ParseUncompressedPublicKey(curve, []byte(a))
		return cryptoFlag(err == nil && ecdsa.VerifyASN1(k, []byte(b), []byte(c))), true
	}
	return "", false
}

// cryptoX509 is x509.xo's op.
func cryptoX509(what string, a, b Bytes) (Bytes, bool) {
	switch what {
	case "parse":
		c, err := x509.ParseCertificate([]byte(a))
		if err != nil {
			return "error", true
		}
		return Bytes(describeCert(c)), true
	case "signed_by":
		c, err := x509.ParseCertificate([]byte(a))
		p, err2 := x509.ParseCertificate([]byte(b))
		return cryptoFlag(err == nil && err2 == nil && p.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil), true
	}
	return "", false
}

// cryptoChain is std/native/llrt/x509verify.xo's op: x509.Verify with
// the outcome as x509verify.xo's code and the chain's common names.
func cryptoChain(what string, a, b, c, d Bytes) (Bytes, bool) {
	if what != "verify" {
		return "", false
	}
	leaf, err := x509.ParseCertificate([]byte(a))
	if err != nil {
		return "parse", true
	}
	args := strings.Split(string(d), " ")
	if len(args) != 3 {
		return "args", true
	}
	now, _ := strconv.ParseInt(args[0], 10, 64)
	opts := x509.VerifyOptions{
		Roots:         chainPool(c),
		Intermediates: chainPool(b),
		CurrentTime:   time.Unix(now, 0),
	}
	if args[1] != "-" {
		opts.DNSName = args[1]
	}
	switch args[2] {
	case "2.5.29.37.0":
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}
	case "1.3.6.1.5.5.7.3.1":
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	case "1.3.6.1.5.5.7.3.2":
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	chains, err := leaf.Verify(opts)
	code := 0
	var ua x509.UnknownAuthorityError
	var ci x509.CertificateInvalidError
	var he x509.HostnameError
	var uc x509.UnhandledCriticalExtension
	switch {
	case err == nil:
	case errors.As(err, &ua):
		code = 1
	case errors.As(err, &ci):
		code = map[x509.InvalidReason]int{x509.Expired: 2, x509.NotAuthorizedToSign: 3, x509.TooManyIntermediates: 4,
			x509.CANotAuthorizedForThisName: 5, x509.IncompatibleUsage: 6}[ci.Reason]
	case errors.As(err, &he):
		code = 7
	case errors.As(err, &uc):
		code = 8
	default:
		return Bytes("error " + err.Error()), true
	}
	out := strconv.Itoa(code)
	if err == nil {
		for _, x := range chains[0] {
			out += " " + x.Subject.CommonName
		}
	}
	return Bytes(out), true
}

// chainPool is the pool of a list of 2 byte big endian lengths and DERs.
func chainPool(b Bytes) *x509.CertPool {
	p := x509.NewCertPool()
	for len(b) >= 2 {
		n := int(b[0])<<8 | int(b[1])
		if c, err := x509.ParseCertificate([]byte(b[2 : 2+n])); err == nil {
			p.AddCert(c)
		}
		b = b[2+n:]
	}
	return p
}

// describeCert is std/native/llrt/x509.xo's describe of c: its fields one
// a line, in the numbering of x509.xo's constants.
func describeCert(c *x509.Certificate) string {
	h := hex.EncodeToString
	sigAlg := map[x509.SignatureAlgorithm]int{
		x509.SHA256WithRSA: 1, x509.SHA384WithRSA: 2, x509.SHA512WithRSA: 3,
		x509.ECDSAWithSHA256: 4, x509.ECDSAWithSHA384: 5, x509.ECDSAWithSHA512: 6,
		x509.PureEd25519: 7, x509.SHA256WithRSAPSS: 8, x509.SHA384WithRSAPSS: 9, x509.SHA512WithRSAPSS: 10,
		x509.MD5WithRSA: 11, x509.SHA1WithRSA: 11, x509.ECDSAWithSHA1: 11, x509.DSAWithSHA1: 11,
	}[c.SignatureAlgorithm]
	keyAlg, key := 0, []byte(nil)
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		keyAlg, key = 1, x509.MarshalPKCS1PublicKey(k)
	case *ecdsa.PublicKey:
		keyAlg = 5
		switch k.Curve {
		case elliptic.P256():
			keyAlg = 2
		case elliptic.P384():
			keyAlg = 3
		}
		key, _ = k.Bytes()
	case ed25519.PublicKey:
		keyAlg, key = 4, k
	}
	var ips []string
	for _, ip := range c.IPAddresses {
		ips = append(ips, h(ip))
	}
	maxPath := c.MaxPathLen
	if !c.BasicConstraintsValid {
		maxPath = -1
	}
	ekuOID := map[x509.ExtKeyUsage]string{
		x509.ExtKeyUsageAny: "2.5.29.37.0", x509.ExtKeyUsageServerAuth: "1.3.6.1.5.5.7.3.1",
		x509.ExtKeyUsageClientAuth: "1.3.6.1.5.5.7.3.2", x509.ExtKeyUsageCodeSigning: "1.3.6.1.5.5.7.3.3",
		x509.ExtKeyUsageEmailProtection: "1.3.6.1.5.5.7.3.4", x509.ExtKeyUsageTimeStamping: "1.3.6.1.5.5.7.3.8",
		x509.ExtKeyUsageOCSPSigning: "1.3.6.1.5.5.7.3.9",
	}
	var eku []string
	for _, u := range c.ExtKeyUsage {
		eku = append(eku, ekuOID[u])
	}
	for _, u := range c.UnknownExtKeyUsage {
		eku = append(eku, u.String())
	}
	j := func(xs []string) string { return strings.Join(xs, ",") }
	var b strings.Builder
	fmt.Fprintf(&b, "version %d\nserial %s\nsig_alg %d\nsignature %s\n", c.Version, h(c.SerialNumber.Bytes()), sigAlg, h(c.Signature))
	fmt.Fprintf(&b, "issuer %s\nsubject %s\ncn %s\n", h(c.RawIssuer), h(c.RawSubject), c.Subject.CommonName)
	fmt.Fprintf(&b, "not_before %d\nnot_after %d\nkey_alg %d\nkey %s\nspki %s\n", c.NotBefore.Unix(), c.NotAfter.Unix(), keyAlg, h(key), h(c.RawSubjectPublicKeyInfo))
	fmt.Fprintf(&b, "dns %s\nips %s\n", j(c.DNSNames), j(ips))
	fmt.Fprintf(&b, "ca %t bc %t max_path %d\nkey_usage %d\n", c.IsCA, c.BasicConstraintsValid, maxPath, c.KeyUsage)
	fmt.Fprintf(&b, "ext_usage %s\nski %s\naki %s\n", j(eku), h(c.SubjectKeyId), h(c.AuthorityKeyId))
	fmt.Fprintf(&b, "permitted %s\nexcluded %s\nunhandled %d\n", j(c.PermittedDNSDomains), j(c.ExcludedDNSDomains), len(c.UnhandledCriticalExtensions))
	fmt.Fprintf(&b, "tbs %d\n", len(c.RawTBSCertificate))
	return b.String()
}

// cryptoRSA is rsa.xo's op.
func cryptoRSA(what string, a, b, c, d Bytes) (Bytes, bool) {
	h := crypto.SHA256
	if len(a) > 0 && a[0] == 1 {
		h = crypto.SHA384
	} else if len(a) > 0 && a[0] == 2 {
		h = crypto.SHA512
	}
	priv := func(der Bytes) *rsa.PrivateKey {
		if k, err := x509.ParsePKCS1PrivateKey([]byte(der)); err == nil {
			return k
		}
		if k, err := x509.ParsePKCS8PrivateKey([]byte(der)); err == nil {
			if r, ok := k.(*rsa.PrivateKey); ok {
				return r
			}
		}
		return nil
	}
	switch what {
	case "public":
		k := priv(a)
		if k == nil {
			return "", true
		}
		return Bytes(x509.MarshalPKCS1PublicKey(&k.PublicKey)), true
	case "verify_pkcs1", "verify_pss":
		k, err := x509.ParsePKCS1PublicKey([]byte(b))
		if err != nil {
			return cryptoFlag(false), true
		}
		if what == "verify_pss" {
			return cryptoFlag(rsa.VerifyPSS(k, h, []byte(c), []byte(d), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) == nil), true
		}
		return cryptoFlag(rsa.VerifyPKCS1v15(k, h, []byte(c), []byte(d)) == nil), true
	case "sign_pkcs1", "sign_pss":
		k := priv(b)
		if k == nil {
			return "", true
		}
		var sig []byte
		var err error
		if what == "sign_pss" {
			sig, err = rsa.SignPSS(rand.Reader, k, h, []byte(c), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
		} else {
			sig, err = rsa.SignPKCS1v15(rand.Reader, k, h, []byte(c))
		}
		if err != nil {
			return "", true
		}
		return Bytes(sig), true
	}
	return "", false
}

// cryptoAES is aesgcm.xo's op.
func cryptoAES(what string, a, b, c, d Bytes) (Bytes, bool) {
	blk, err := aes.NewCipher([]byte(a))
	if err != nil {
		panic(err)
	}
	if what == "block" {
		out := make([]byte, 16)
		blk.Encrypt(out, []byte(b))
		return Bytes(out), true
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		panic(err)
	}
	switch what {
	case "seal":
		return Bytes(g.Seal(nil, []byte(b), []byte(c), []byte(d))), true
	case "open":
		p, err := g.Open(nil, []byte(b), []byte(c), []byte(d))
		if err != nil {
			return "\x00", true
		}
		return "\x01" + Bytes(p), true
	}
	return "", false
}

// cryptoChaCha is chacha.xo's op. Go's standard library has no
// ChaCha20-Poly1305 (golang.org/x/crypto does), so this is RFC 8439
// written out, Poly1305 with math/big (the reference, not constant time).
func cryptoChaCha(what string, a, b, c, d Bytes) (Bytes, bool) {
	switch what {
	case "block":
		return Bytes(chachaBlock([]byte(a), uint32(cryptoNum(c)), []byte(b))), true
	case "xor":
		return Bytes(chachaXor([]byte(a), uint32(cryptoNum(c)), []byte(b), []byte(d))), true
	case "poly1305":
		return Bytes(poly1305([]byte(a), []byte(b))), true
	case "seal":
		ct := chachaXor([]byte(a), 1, []byte(b), []byte(c))
		return Bytes(ct) + Bytes(chachaTag([]byte(a), []byte(b), ct, []byte(d))), true
	case "open":
		n := len(c) - 16
		if n < 0 {
			return "\x00", true
		}
		ct := []byte(c[:n])
		if subtle.ConstantTimeCompare(chachaTag([]byte(a), []byte(b), ct, []byte(d)), []byte(c[n:])) != 1 {
			return "\x00", true
		}
		return "\x01" + Bytes(chachaXor([]byte(a), 1, []byte(b), ct)), true
	}
	return "", false
}

func chachaBlock(key []byte, ctr uint32, nonce []byte) []byte {
	var s, x [16]uint32
	s[0], s[1], s[2], s[3] = 0x61707865, 0x3320646e, 0x79622d32, 0x6b206574
	for i := 0; i < 8; i++ {
		s[4+i] = binary.LittleEndian.Uint32(key[4*i:])
	}
	s[12] = ctr
	for i := 0; i < 3; i++ {
		s[13+i] = binary.LittleEndian.Uint32(nonce[4*i:])
	}
	x = s
	qr := func(a, b, c, d int) {
		x[a] += x[b]
		x[d] = bits.RotateLeft32(x[d]^x[a], 16)
		x[c] += x[d]
		x[b] = bits.RotateLeft32(x[b]^x[c], 12)
		x[a] += x[b]
		x[d] = bits.RotateLeft32(x[d]^x[a], 8)
		x[c] += x[d]
		x[b] = bits.RotateLeft32(x[b]^x[c], 7)
	}
	for i := 0; i < 10; i++ {
		qr(0, 4, 8, 12)
		qr(1, 5, 9, 13)
		qr(2, 6, 10, 14)
		qr(3, 7, 11, 15)
		qr(0, 5, 10, 15)
		qr(1, 6, 11, 12)
		qr(2, 7, 8, 13)
		qr(3, 4, 9, 14)
	}
	out := make([]byte, 64)
	for i := range x {
		binary.LittleEndian.PutUint32(out[4*i:], x[i]+s[i])
	}
	return out
}

func chachaXor(key []byte, ctr uint32, nonce, src []byte) []byte {
	out := make([]byte, len(src))
	for i := 0; i < len(src); i += 64 {
		ks := chachaBlock(key, ctr, nonce)
		ctr++
		for j := i; j < len(src) && j < i+64; j++ {
			out[j] = src[j] ^ ks[j-i]
		}
	}
	return out
}

// leInt is the little endian integer of b.
func leInt(b []byte) *big.Int {
	r := make([]byte, len(b))
	for i := range b {
		r[len(b)-1-i] = b[i]
	}
	return new(big.Int).SetBytes(r)
}

func poly1305(key, msg []byte) []byte {
	rb := append([]byte(nil), key[:16]...)
	for _, i := range []int{3, 7, 11, 15} {
		rb[i] &= 15
	}
	for _, i := range []int{4, 8, 12} {
		rb[i] &= 252
	}
	r := leInt(rb)
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 130), big.NewInt(5))
	acc := new(big.Int)
	for i := 0; i < len(msg); i += 16 {
		blk := append([]byte(nil), msg[i:min(i+16, len(msg))]...)
		acc.Add(acc, leInt(append(blk, 1)))
		acc.Mul(acc, r)
		acc.Mod(acc, p)
	}
	acc.Add(acc, leInt(key[16:32]))
	be := acc.FillBytes(make([]byte, 32))
	out := make([]byte, 16)
	for i := range out {
		out[i] = be[31-i]
	}
	return out
}

func chachaTag(key, nonce, ct, aad []byte) []byte {
	var m []byte
	pad := func(b []byte) {
		m = append(m, b...)
		m = append(m, make([]byte, (16-len(b)%16)%16)...)
	}
	pad(aad)
	pad(ct)
	m = binary.LittleEndian.AppendUint64(m, uint64(len(aad)))
	m = binary.LittleEndian.AppendUint64(m, uint64(len(ct)))
	return poly1305(chachaBlock(key, 0, nonce)[:32], m)
}

// cryptoX25519 is x25519.xo's op: all zeros where Go's ECDH reports a
// low order point.
func cryptoX25519(what string, a, b Bytes) (Bytes, bool) {
	k, err := ecdh.X25519().NewPrivateKey([]byte(a))
	if err != nil {
		panic(err)
	}
	switch what {
	case "base":
		return Bytes(k.PublicKey().Bytes()), true
	case "mult":
		p, err := ecdh.X25519().NewPublicKey([]byte(b))
		if err != nil {
			panic(err)
		}
		s, err := k.ECDH(p)
		if err != nil {
			return Bytes(make([]byte, 32)), true
		}
		return Bytes(s), true
	}
	return "", false
}

// cryptoTLS13 is std/native/llrt/tls13.xo's replay: a TLS 1.3 client
// with a fixed ClientHello (a) and ECDHE secret (c) over the parts of b
// (3 byte lengths: the server's handshake records, its later records,
// the client's data, the root's DER), d "time host suite chunk key" (key
// "x25519" when c is the client's X25519 private key). Written
// out for RFC 8448's trace (one suite, an RSA-PSS or ECDSA P-256 server,
// no HelloRetryRequest), with crypto/tls's error texts; the chunk size
// does not change the result.
func cryptoTLS13(what string, a, b, c, d Bytes) (Bytes, bool) {
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
	if len(parts) != 4 || len(args) != 5 {
		return "args", true
	}
	t := &tls13Replay{shared: []byte(c), host: args[1], suite: args[2], x25519: args[4] == "x25519"}
	t.now, _ = strconv.ParseInt(args[0], 10, 64)
	if len(parts[3]) > 0 {
		root, err := x509.ParseCertificate(parts[3])
		if err != nil {
			return "parse", true
		}
		t.roots = append(t.roots, root)
	}
	hello := []byte(a)
	t.tr = append(t.tr, hello...)
	if len(hello) > 38 {
		t.sid = hello[39 : 39+int(hello[38])]
	}
	out := "ch " + hex.EncodeToString(tls13Record(22, 0x0301, hello)) + "\n"
	t.handshake(parts[0])
	out += fmt.Sprintf("flight %d %s %s\n", t.state, hex.EncodeToString(t.out), t.err)
	t.out = nil
	if t.state == 6 {
		out += "app " + hex.EncodeToString(t.w.seal(23, parts[2])) + "\n"
		t.after(parts[1])
		out += fmt.Sprintf("got %s closed %v %s %s\n", hex.EncodeToString(t.app), t.closed, hex.EncodeToString(t.out), t.err)
		out += "bye " + hex.EncodeToString(t.w.seal(21, []byte{1, 0})) + "\n"
	}
	return Bytes(out), true
}

type tls13Replay struct {
	shared, sid, tr, out, app []byte
	hs, cHS, sHS              []byte
	leaf                      *x509.Certificate
	host, suite, err          string
	now                       int64
	roots                     []*x509.Certificate
	state                     int
	closed, x25519            bool
	r, w                      *tls13Keys
}

type tls13Keys struct {
	aead   cipher.AEAD
	iv     []byte
	secret []byte
	seq    uint64
}

func tls13Record(ct byte, vers int, data []byte) []byte {
	return append([]byte{ct, byte(vers >> 8), byte(vers), byte(len(data) >> 8), byte(len(data))}, data...)
}

func tls13ExpandLabel(secret []byte, label string, context []byte, n int) []byte {
	info := []byte{byte(n >> 8), byte(n), byte(6 + len(label))}
	info = append(info, "tls13 "+label...)
	info = append(info, byte(len(context)))
	info = append(info, context...)
	out, _ := hkdf.Expand(sha256.New, secret, string(info), n)
	return out
}

func tls13Hash(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func tls13NewKeys(secret []byte) *tls13Keys {
	blk, _ := aes.NewCipher(tls13ExpandLabel(secret, "key", nil, 16))
	g, _ := cipher.NewGCM(blk)
	return &tls13Keys{aead: g, iv: tls13ExpandLabel(secret, "iv", nil, 12), secret: secret}
}

func (k *tls13Keys) nonce() []byte {
	n := append([]byte(nil), k.iv...)
	var s [8]byte
	binary.BigEndian.PutUint64(s[:], k.seq)
	for i := range s {
		n[4+i] ^= s[i]
	}
	k.seq++
	return n
}

func (k *tls13Keys) seal(ct byte, data []byte) []byte {
	hdr := []byte{23, 3, 3, byte((len(data) + 17) >> 8), byte(len(data) + 17)}
	return k.aead.Seal(hdr, k.nonce(), append(append([]byte(nil), data...), ct), hdr)
}

func (k *tls13Keys) open(hdr, body []byte) (byte, []byte, bool) {
	pt, err := k.aead.Open(nil, k.nonce(), body, hdr)
	if err != nil {
		return 0, nil, false
	}
	i := len(pt) - 1
	for i >= 0 && pt[i] == 0 {
		i--
	}
	if i < 0 {
		return 0, nil, false
	}
	return pt[i], pt[:i], true
}

// fail records the first error and sends the fatal alert.
func (t *tls13Replay) fail(alert byte, msg string) {
	if t.state == 7 {
		return
	}
	t.state, t.err = 7, msg
	if t.w != nil {
		t.out = append(t.out, t.w.seal(21, []byte{2, alert})...)
	} else {
		t.out = append(t.out, tls13Record(21, 0x0303, []byte{2, alert})...)
	}
}

// records splits data into records.
func tls13Records(data []byte) (out [][]byte) {
	for len(data) >= 5 {
		n := 5 + (int(data[3])<<8 | int(data[4]))
		out = append(out, data[:n])
		data = data[n:]
	}
	return out
}

func (t *tls13Replay) handshake(data []byte) {
	var hs []byte
	for _, rec := range tls13Records(data) {
		if t.state == 7 {
			return
		}
		switch {
		case rec[0] == 22 && t.r == nil:
			t.serverHello(rec[5:])
		case rec[0] == 23 && t.r != nil:
			ct, pt, ok := t.r.open(rec[:5], rec[5:])
			if !ok {
				t.fail(20, "local error: tls: bad record MAC")
				return
			}
			if ct != 22 {
				t.fail(10, "local error: tls: unexpected message")
				return
			}
			hs = append(hs, pt...)
			for len(hs) >= 4 && t.state != 7 {
				n := 4 + (int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3]))
				if len(hs) < n {
					break
				}
				t.message(hs[:n])
				hs = hs[n:]
			}
		}
	}
}

func (t *tls13Replay) serverHello(m []byte) {
	sn := int(m[38])
	suite := hex.EncodeToString(m[39+sn : 41+sn])
	if suite != t.suite {
		t.fail(47, "tls: server chose an unconfigured cipher suite")
		return
	}
	if t.x25519 {
		// the server's key share: the extensions after the compression byte
		var share []byte
		for e := m[44+sn:]; len(e) >= 4; {
			typ, n := int(e[0])<<8|int(e[1]), int(e[2])<<8|int(e[3])
			if typ == 51 && n >= 4 {
				share = e[8 : 4+n]
			}
			e = e[4+n:]
		}
		priv, err := ecdh.X25519().NewPrivateKey(t.shared)
		if err != nil {
			panic(err)
		}
		pub, err := ecdh.X25519().NewPublicKey(share)
		if err != nil {
			t.fail(47, "tls: invalid server key share")
			return
		}
		if t.shared, err = priv.ECDH(pub); err != nil {
			t.fail(47, "tls: invalid server key share")
			return
		}
	}
	t.tr = append(t.tr, m...)
	zero := make([]byte, 32)
	early, _ := hkdf.Extract(sha256.New, zero, nil)
	derived := tls13ExpandLabel(early, "derived", tls13Hash(nil), 32)
	hs, _ := hkdf.Extract(sha256.New, t.shared, derived)
	th := tls13Hash(t.tr)
	t.hs = hs
	t.cHS = tls13ExpandLabel(hs, "c hs traffic", th, 32)
	t.sHS = tls13ExpandLabel(hs, "s hs traffic", th, 32)
	t.w = tls13NewKeys(t.cHS)
	t.r = tls13NewKeys(t.sHS)
	t.state = 1
}

func (t *tls13Replay) message(m []byte) {
	switch {
	case m[0] == 8 && t.state == 1:
		t.state = 2
	case m[0] == 11 && t.state == 2:
		// one certificate entry: context 0, list length, entry length
		n := int(m[8])<<16 | int(m[9])<<8 | int(m[10])
		leaf, err := x509.ParseCertificate(m[11 : 11+n])
		if err != nil {
			t.fail(42, "tls: failed to parse certificate from server: "+err.Error())
			return
		}
		pool := x509.NewCertPool()
		for _, r := range t.roots {
			pool.AddCert(r)
		}
		opts := x509.VerifyOptions{Roots: pool, CurrentTime: time.Unix(t.now, 0).UTC(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		if t.host != "-" {
			opts.DNSName = t.host
		}
		if _, err := leaf.Verify(opts); err != nil {
			alert := byte(42)
			var ua x509.UnknownAuthorityError
			var ci x509.CertificateInvalidError
			if errors.As(err, &ua) {
				alert = 48
			} else if errors.As(err, &ci) && ci.Reason == x509.Expired {
				alert = 45
			}
			t.fail(alert, "tls: failed to verify certificate: "+err.Error())
			return
		}
		t.leaf = leaf
		t.state = 4
	case m[0] == 15 && t.state == 4:
		scheme := int(m[4])<<8 | int(m[5])
		sig := m[8:]
		content := append(bytes64(0x20), "TLS 1.3, server CertificateVerify\x00"...)
		digest := tls13Hash(append(content, tls13Hash(t.tr)...))
		ok := false
		switch k := t.leaf.PublicKey.(type) {
		case *rsa.PublicKey:
			ok = scheme == 0x0804 && rsa.VerifyPSS(k, crypto.SHA256, digest, sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) == nil
		case *ecdsa.PublicKey:
			ok = scheme == 0x0403 && ecdsa.VerifyASN1(k, digest, sig)
		}
		if !ok {
			why := "crypto/rsa: verification error"
			if _, ec := t.leaf.PublicKey.(*ecdsa.PublicKey); ec {
				why = "ECDSA verification failure"
			}
			t.fail(51, "tls: invalid signature by the server certificate: "+why)
			return
		}
		t.state = 5
	case m[0] == 20 && t.state == 5:
		if !hmac.Equal(m[4:], tls13Finished(t.sHS, t.tr)) {
			t.fail(51, "tls: invalid server finished hash")
			return
		}
		t.tr = append(t.tr, m...)
		derived := tls13ExpandLabel(t.hs, "derived", tls13Hash(nil), 32)
		master, _ := hkdf.Extract(sha256.New, make([]byte, 32), derived)
		th := tls13Hash(t.tr)
		cAP := tls13ExpandLabel(master, "c ap traffic", th, 32)
		t.r = tls13NewKeys(tls13ExpandLabel(master, "s ap traffic", th, 32))
		fin := append([]byte{20, 0, 0, 32}, tls13Finished(t.cHS, t.tr)...)
		t.out = append(t.out, t.w.seal(22, fin)...)
		t.w = tls13NewKeys(cAP)
		t.state = 6
		return
	default:
		t.fail(10, "tls: unexpected message")
		return
	}
	t.tr = append(t.tr, m...)
}

func bytes64(c byte) []byte {
	b := make([]byte, 64)
	for i := range b {
		b[i] = c
	}
	return b
}

func tls13Finished(base, tr []byte) []byte {
	m := hmac.New(sha256.New, tls13ExpandLabel(base, "finished", nil, 32))
	m.Write(tls13Hash(tr))
	return m.Sum(nil)
}

// after takes the server's records after the handshake: tickets dropped,
// data kept, close_notify closing.
func (t *tls13Replay) after(data []byte) {
	for _, rec := range tls13Records(data) {
		if t.state == 7 || t.closed {
			return
		}
		ct, pt, ok := t.r.open(rec[:5], rec[5:])
		if !ok {
			t.fail(20, "local error: tls: bad record MAC")
			return
		}
		switch {
		case ct == 23:
			t.app = append(t.app, pt...)
		case ct == 21 && len(pt) == 2 && pt[1] == 0:
			t.closed = true
		case ct == 22 && pt[0] == 4:
		default:
			t.fail(10, "tls: unexpected message")
		}
	}
}
