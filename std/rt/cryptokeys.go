package rt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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
