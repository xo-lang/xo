package rt

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strconv"
	"strings"
)

// std/crypto's public key signatures, keys, and PEM (decision 0285): Go's
// packages, as std/native/crypto.xo ports them. Outside bytes become Str
// through textOf (decision 0259).

func (e Crypto_KeyErr) M_display(*Task) string { return e.F_msg }

func Crypto_pem_decode(tk *Task, text string) List[Crypto_PemBlock] {
	var out []Crypto_PemBlock
	rest := []byte(text)
	for {
		b, r := pem.Decode(rest)
		if b == nil {
			break
		}
		var hs Map[string, string]
		for k, v := range b.Headers {
			Map_put(tk, &hs, textOf(k), textOf(v))
		}
		out = append(out, Crypto_PemBlock{F_kind: textOf(b.Type), F_headers: hs, F_bytes: Bytes(b.Bytes)})
		rest = r
	}
	return ListFromSlice(out)
}

func Crypto_pem_encode(_ *Task, b Crypto_PemBlock) Result[string, Crypto_KeyErr] {
	h := map[string]string{}
	for k, v := range Map_All(b.F_headers) {
		h[k] = v
	}
	e := pem.EncodeToMemory(&pem.Block{Type: b.F_kind, Headers: h, Bytes: []byte(b.F_bytes)})
	if e == nil {
		return Err[string](Crypto_KeyErr{F_msg: "pem: cannot encode a header key that contains a colon"})
	}
	return Ok[string, Crypto_KeyErr](string(e))
}

// Crypto_PublicKey is `crypto.PublicKey`: Go's key and its PKIX DER, which
// == and hashing compare.
type Crypto_PublicKey struct {
	k   any
	der string
}

func (v Crypto_PublicKey) XoEq(x any) bool {
	o, ok := x.(Crypto_PublicKey)
	return ok && o.der == v.der
}

func (v Crypto_PublicKey) XoHash() uint64 { return Hash(v.der) }

func (Crypto_PublicKey) XoDebug(b *strings.Builder) { b.WriteString("<publickey>") }

func (v Crypto_PublicKey) M_debug(*Task) string { return Debug(v) }

func (v Crypto_PublicKey) M_alg(*Task) Crypto_KeyAlg {
	switch k := v.k.(type) {
	case *rsa.PublicKey:
		return Crypto_KeyAlg{Tag: 1}
	case *ecdsa.PublicKey:
		return Crypto_KeyAlg{Tag: map[string]int{"P-224": 2, "P-256": 3, "P-384": 4, "P-521": 5}[k.Curve.Params().Name]}
	}
	return Crypto_KeyAlg{Tag: 6}
}

func keyErr(m string) Crypto_KeyErr { return Crypto_KeyErr{F_msg: m} }

// Crypto_parse_public_key is x509.ParsePKIXPublicKey, else
// ParsePKCS1PublicKey; encoding/asn1's errors are `x509: malformed public
// key`, and X25519 and DSA keys `x509: unknown public key algorithm`.
func Crypto_parse_public_key(_ *Task, der Bytes) Result[Crypto_PublicKey, Crypto_KeyErr] {
	k, err := x509.ParsePKIXPublicKey([]byte(der))
	if err != nil && strings.Contains(err.Error(), "use ParsePKCS1PublicKey") {
		var pk *rsa.PublicKey
		pk, err = x509.ParsePKCS1PublicKey([]byte(der))
		if err == nil {
			k = pk
		}
	}
	if err != nil {
		m := err.Error()
		if strings.HasPrefix(m, "asn1:") {
			m = "x509: malformed public key"
		}
		return Err[Crypto_PublicKey](keyErr(m))
	}
	switch k.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey, ed25519.PublicKey:
	default:
		return Err[Crypto_PublicKey](keyErr("x509: unknown public key algorithm"))
	}
	m, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		return Err[Crypto_PublicKey](keyErr(err.Error()))
	}
	return Ok[Crypto_PublicKey, Crypto_KeyErr](Crypto_PublicKey{k: k, der: string(m)})
}

func Crypto_marshal_public_key(_ *Task, k Crypto_PublicKey) Bytes { return Bytes(k.der) }

// Crypto_PrivateKey is `crypto.PrivateKey`: Go's key and its public key.
// Debug never shows key material.
type Crypto_PrivateKey struct {
	k   any
	pub Crypto_PublicKey
}

func (v Crypto_PrivateKey) XoEq(x any) bool {
	o, ok := x.(Crypto_PrivateKey)
	return ok && o.pub.der == v.pub.der
}

func (v Crypto_PrivateKey) XoHash() uint64 { return Hash(v.pub.der) }

func (Crypto_PrivateKey) XoDebug(b *strings.Builder) { b.WriteString("<privatekey>") }

func (v Crypto_PrivateKey) M_debug(*Task) string { return Debug(v) }

func (v Crypto_PrivateKey) M_alg(tk *Task) Crypto_KeyAlg { return v.pub.M_alg(tk) }

func (v Crypto_PrivateKey) M_public_key(*Task) Crypto_PublicKey { return v.pub }

// Crypto_parse_private_key is x509.ParsePKCS8PrivateKey, following its
// hints to ParsePKCS1PrivateKey and ParseECPrivateKey; encoding/asn1's
// texts become `x509: malformed private key` (alone) or `malformed DER`
// (after another message), X25519 keys PKCS #8's unknown algorithm, and
// keys of more than two primes `crypto/rsa: multi-prime keys are not
// supported`, as the native code says.
func Crypto_parse_private_key(_ *Task, der Bytes) Result[Crypto_PrivateKey, Crypto_KeyErr] {
	b := []byte(der)
	k, err := x509.ParsePKCS8PrivateKey(b)
	if err != nil {
		m := err.Error()
		if strings.Contains(m, "use ParsePKCS1PrivateKey") {
			k, err = x509.ParsePKCS1PrivateKey(b)
		} else if strings.Contains(m, "use ParseECPrivateKey") {
			k, err = x509.ParseECPrivateKey(b)
		}
	}
	if err != nil {
		m := err.Error()
		if strings.HasPrefix(m, "asn1:") {
			m = "x509: malformed private key"
		} else if i := strings.Index(m, "asn1:"); i >= 0 {
			m = m[:i] + "malformed DER"
		}
		return Err[Crypto_PrivateKey](keyErr(textOf(m)))
	}
	var pub any
	switch x := k.(type) {
	case *rsa.PrivateKey:
		if len(x.Primes) > 2 {
			return Err[Crypto_PrivateKey](keyErr("crypto/rsa: multi-prime keys are not supported"))
		}
		pub = &x.PublicKey
	case *ecdsa.PrivateKey:
		pub = &x.PublicKey
	case ed25519.PrivateKey:
		pub = x.Public()
	default:
		return Err[Crypto_PrivateKey](keyErr("x509: PKCS#8 wrapping contained private key with unknown algorithm: 1.3.101.110"))
	}
	m, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return Err[Crypto_PrivateKey](keyErr(err.Error()))
	}
	return Ok[Crypto_PrivateKey, Crypto_KeyErr](Crypto_PrivateKey{k: k, pub: Crypto_PublicKey{k: pub, der: string(m)}})
}

// The crypto.Hash of a std/crypto Hash (tags 1 to 8).
var cryptoGoHashes = [...]crypto.Hash{0, crypto.MD5, crypto.SHA1, crypto.SHA224, crypto.SHA256, crypto.SHA384, crypto.SHA512, crypto.SHA3_256, crypto.SHA3_512}

func keySum(h Crypto_Hash, msg Bytes) []byte {
	x := cryptoHashes[h.Tag]()
	x.Write([]byte(msg))
	return x.Sum(nil)
}

func Crypto_verify(_ *Task, k Crypto_PublicKey, h Crypto_Hash, msg Bytes, sig Bytes) bool {
	switch p := k.k.(type) {
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(p, cryptoGoHashes[h.Tag], keySum(h, msg), []byte(sig)) == nil
	case *ecdsa.PublicKey:
		return ecdsa.VerifyASN1(p, keySum(h, msg), []byte(sig))
	case ed25519.PublicKey:
		return ed25519.Verify(p, []byte(msg), []byte(sig))
	}
	return false
}

func Crypto_verify_pss(_ *Task, k Crypto_PublicKey, h Crypto_Hash, msg Bytes, sig Bytes) bool {
	p, ok := k.k.(*rsa.PublicKey)
	return ok && rsa.VerifyPSS(p, cryptoGoHashes[h.Tag], keySum(h, msg), []byte(sig), nil) == nil
}

func Crypto_verify_raw_ecdsa(_ *Task, k Crypto_PublicKey, h Crypto_Hash, msg Bytes, sig Bytes) bool {
	p, ok := k.k.(*ecdsa.PublicKey)
	if !ok {
		return false
	}
	size := (p.Curve.Params().N.BitLen() + 7) / 8
	if len(sig) != 2*size {
		return false
	}
	return ecdsa.Verify(p, keySum(h, msg), new(big.Int).SetBytes([]byte(sig[:size])), new(big.Int).SetBytes([]byte(sig[size:])))
}

// Crypto_sign: Go 1.26's signers read their random input from the system
// generator (a custom reader is ignored), so ECDSA signatures here are
// hedged but not reproducible from r; the bytes the native signer takes
// from r are drawn and dropped, so r's later values match the native
// backends'.
func Crypto_sign(tk *Task, r Rand, k Crypto_PrivateKey, h Crypto_Hash, msg Bytes) Result[Bytes, Crypto_KeyErr] {
	var sig []byte
	var err error
	switch p := k.k.(type) {
	case *rsa.PrivateKey:
		sig, err = rsa.SignPKCS1v15(nil, p, cryptoGoHashes[h.Tag], keySum(h, msg))
	case *ecdsa.PrivateKey:
		r.M_bytes(tk, int64((p.Curve.Params().N.BitLen()+7)/8))
		sig, err = ecdsa.SignASN1(cryptorand.Reader, p, keySum(h, msg))
	case ed25519.PrivateKey:
		sig = ed25519.Sign(p, []byte(msg))
	}
	if err != nil {
		return Err[Bytes](keyErr(err.Error()))
	}
	return Ok[Bytes, Crypto_KeyErr](Bytes(sig))
}

func Crypto_sign_pss(tk *Task, r Rand, k Crypto_PrivateKey, h Crypto_Hash, msg Bytes) Result[Bytes, Crypto_KeyErr] {
	p, ok := k.k.(*rsa.PrivateKey)
	if !ok {
		return Err[Bytes](keyErr("crypto: sign_pss needs an RSA key"))
	}
	r.M_bytes(tk, int64(cryptoGoHashes[h.Tag].Size()))
	sig, err := rsa.SignPSS(cryptorand.Reader, p, cryptoGoHashes[h.Tag], keySum(h, msg), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	if err != nil {
		return Err[Bytes](keyErr(err.Error()))
	}
	return Ok[Bytes, Crypto_KeyErr](Bytes(sig))
}

func Crypto_sign_raw_ecdsa(tk *Task, r Rand, k Crypto_PrivateKey, h Crypto_Hash, msg Bytes) Result[Bytes, Crypto_KeyErr] {
	p, ok := k.k.(*ecdsa.PrivateKey)
	if !ok {
		return Err[Bytes](keyErr("crypto: sign_raw_ecdsa needs an ECDSA key"))
	}
	d := Crypto_sign(tk, r, k, h, msg)
	if !d.Ok {
		return d
	}
	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal([]byte(d.V), &sig); err != nil {
		return Err[Bytes](keyErr(err.Error()))
	}
	size := (p.Curve.Params().N.BitLen() + 7) / 8
	out := make([]byte, 2*size)
	sig.R.FillBytes(out[:size])
	sig.S.FillBytes(out[size:])
	return Ok[Bytes, Crypto_KeyErr](Bytes(out))
}

// Crypto_generate_key reads r as the native code does: an ECDSA scalar by
// Go's rejection sampling (P-521's candidates shifted right by 7 bits),
// an Ed25519 seed of 32 bytes; so the same r gives the same key on every
// backend.
func Crypto_generate_key(tk *Task, r Rand, alg Crypto_KeyAlg) Result[Crypto_PrivateKey, Crypto_KeyErr] {
	var k any
	switch alg.Tag {
	case 1:
		return Err[Crypto_PrivateKey](keyErr("crypto: RSA key generation is not available yet (decision 0285)"))
	case 6:
		k = ed25519.NewKeyFromSeed([]byte(r.M_bytes(tk, 32)))
	default:
		c := []elliptic.Curve{nil, nil, elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()}[alg.Tag]
		size := (c.Params().N.BitLen() + 7) / 8
		for {
			b := []byte(r.M_bytes(tk, int64(size)))
			if c == elliptic.P521() {
				for i := len(b) - 1; i >= 0; i-- {
					b[i] >>= 7
					if i > 0 {
						b[i] |= b[i-1] << 1
					}
				}
			}
			if ek, err := ecdsa.ParseRawPrivateKey(c, b); err == nil {
				k = ek
				break
			}
		}
	}
	s := k.(crypto.Signer)
	m, err := x509.MarshalPKIXPublicKey(s.Public())
	if err != nil {
		return Err[Crypto_PrivateKey](keyErr(err.Error()))
	}
	return Ok[Crypto_PrivateKey, Crypto_KeyErr](Crypto_PrivateKey{k: k, pub: Crypto_PublicKey{k: s.Public(), der: string(m)}})
}

// Crypto_jwk_public_key follows the native code's rules and texts: the
// members matched exactly (encoding/json alone would match them without
// case), each a string or null.
func Crypto_jwk_public_key(_ *Task, jwk string) Result[Crypto_PublicKey, Crypto_KeyErr] {
	fail := func(m string) Result[Crypto_PublicKey, Crypto_KeyErr] { return Err[Crypto_PublicKey](keyErr(m)) }
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jwk), &raw); err != nil || raw == nil {
		return fail("jwk: malformed JSON")
	}
	mem := map[string]*string{}
	for _, name := range []string{"kty", "crv", "n", "e", "x", "y"} {
		v, ok := raw[name]
		if !ok || string(v) == "null" {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return fail("jwk: malformed JSON")
		}
		mem[name] = &s
	}
	if mem["kty"] == nil {
		return fail("jwk: malformed JSON")
	}
	b64 := func(name string) ([]byte, string) {
		v := mem[name]
		if v == nil {
			return nil, "jwk: missing member \"" + name + "\""
		}
		b, err := base64.RawURLEncoding.DecodeString(*v)
		if err != nil {
			return nil, "jwk: member \"" + name + "\" is not unpadded base64url"
		}
		return b, ""
	}
	crv := ""
	if mem["crv"] != nil {
		crv = *mem["crv"]
	}
	var k any
	switch kty := *mem["kty"]; kty {
	case "RSA":
		n, e1 := b64("n")
		if e1 != "" {
			return fail(e1)
		}
		e, e2 := b64("e")
		if e2 != "" {
			return fail(e2)
		}
		e = bytes.TrimLeft(e, "\x00")
		nn := new(big.Int).SetBytes(n)
		ev := new(big.Int).SetBytes(e)
		if len(e) > 4 || nn.Sign() == 0 || ev.Sign() == 0 {
			return fail("jwk: invalid RSA key")
		}
		k = &rsa.PublicKey{N: nn, E: int(ev.Int64())}
	case "EC":
		curves := map[string]elliptic.Curve{"P-256": elliptic.P256(), "P-384": elliptic.P384(), "P-521": elliptic.P521()}
		c, ok := curves[crv]
		if !ok {
			return fail("jwk: unsupported curve " + strconv.Quote(crv))
		}
		size := (c.Params().BitSize + 7) / 8
		x, e1 := b64("x")
		if e1 != "" {
			return fail(e1)
		}
		y, e2 := b64("y")
		if e2 != "" {
			return fail(e2)
		}
		if len(x) != size || len(y) != size {
			return fail("jwk: coordinates of the wrong size for " + crv)
		}
		p, err := ecdsa.ParseUncompressedPublicKey(c, append(append([]byte{4}, x...), y...))
		if err != nil {
			return fail("jwk: point not on the curve")
		}
		k = p
	case "OKP":
		if crv != "Ed25519" {
			return fail("jwk: unsupported curve " + strconv.Quote(crv))
		}
		x, e1 := b64("x")
		if e1 != "" {
			return fail(e1)
		}
		if len(x) != 32 {
			return fail("jwk: Ed25519 key of the wrong size")
		}
		k = ed25519.PublicKey(x)
	default:
		return fail("jwk: unsupported key type " + strconv.Quote(kty))
	}
	m, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		return fail(err.Error())
	}
	return Ok[Crypto_PublicKey, Crypto_KeyErr](Crypto_PublicKey{k: k, der: string(m)})
}
