package rt

import (
	"crypto"
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
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
	"strings"
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
