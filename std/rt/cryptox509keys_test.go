//go:build !xo_no_crypto

package rt

import (
	"crypto"
	"crypto/dsa"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
)

// x5canon is a key as text, to compare a key parsed by the port with
// one parsed by crypto/x509.
func x5canon(k any) string {
	switch k := k.(type) {
	case *rsa.PrivateKey:
		return "rsa " + hex.EncodeToString(x509.MarshalPKCS1PrivateKey(k))
	case *rsa.PublicKey:
		return "rsapub " + hex.EncodeToString(x509.MarshalPKCS1PublicKey(k))
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		return fmt.Sprintf("ec %x %v", b, err)
	case *ecdsa.PublicKey, ed25519.PublicKey, *ecdh.PublicKey:
		b, err := x509.MarshalPKIXPublicKey(k)
		return fmt.Sprintf("%T %x %v", k, b, err)
	case ed25519.PrivateKey:
		return "ed " + hex.EncodeToString(k)
	case *ecdh.PrivateKey:
		return fmt.Sprintf("ecdh %v %x", k.Curve(), k.Bytes())
	case *dsa.PublicKey:
		return fmt.Sprintf("dsa %v %v %v %v", k.Y, k.P, k.Q, k.G)
	case nil:
		return "nil"
	}
	return fmt.Sprintf("%T %v", k, k)
}

func x5errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// x5mutations is der, every prefix of it, der with each byte changed
// (every byte up to 256, then a sample), and der with trailing data.
func x5mutations(der []byte) [][]byte {
	var out [][]byte
	out = append(out, der)
	for n := 0; n < len(der); n++ {
		out = append(out, der[:n])
	}
	step := 1
	if len(der) > 256 {
		step = len(der) / 128
	}
	for i := 0; i < len(der); i += step {
		for _, x := range []byte{0x00, 0x01, 0x7f, 0x80, 0xff, der[i] ^ 0x01} {
			if x == der[i] {
				continue
			}
			m := append([]byte(nil), der...)
			m[i] = x
			out = append(out, m)
		}
	}
	out = append(out, append(append([]byte(nil), der...), 0x00))
	return out
}

// TestX509KeysMatchGo checks cryptox509keys.go's functions against
// crypto/x509 (task #516): the same bytes from each marshal function and,
// for every key type and corrupted encodings of it, the same key or the
// same error text from each parse function, which std/crypto passes on
// to programs.
func TestX509KeysMatchGo(t *testing.T) {
	rsa1, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	rsa3, err := rsa.GenerateMultiPrimeKey(rand.Reader, 3, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privs := []any{rsa1, rsa3}
	for _, c := range []elliptic.Curve{elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		k, err := ecdsa.GenerateKey(c, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		privs = append(privs, k)
	}
	_, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privs = append(privs, ed)
	for _, c := range []ecdh.Curve{ecdh.X25519(), ecdh.P256()} {
		k, err := c.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		privs = append(privs, k)
	}

	var pubs []any
	for _, k := range privs {
		pubs = append(pubs, k.(interface{ Public() crypto.PublicKey }).Public())
	}
	// a DSA key, which only ParsePKIXPublicKey reads
	var dp dsa.Parameters
	if err := dsa.GenerateParameters(&dp, rand.Reader, dsa.L1024N160); err != nil {
		t.Fatal(err)
	}
	dk := &dsa.PrivateKey{PublicKey: dsa.PublicKey{Parameters: dp}}
	if err := dsa.GenerateKey(dk, rand.Reader); err != nil {
		t.Fatal(err)
	}
	dparams, _ := asn1.Marshal(dsaAlgorithmParameters{P: dp.P, Q: dp.Q, G: dp.G})
	dy, _ := asn1.Marshal(dk.Y)
	dsaDER, _ := asn1.Marshal(pkixPublicKey{
		Algo:      pkix.AlgorithmIdentifier{Algorithm: oidPublicKeyDSA, Parameters: asn1.RawValue{FullBytes: dparams}},
		BitString: asn1.BitString{Bytes: dy, BitLength: 8 * len(dy)},
	})
	// an RSA key without its NULL parameters
	rsaPKCS1 := x509.MarshalPKCS1PublicKey(&rsa1.PublicKey)
	rsaNoNull, _ := asn1.Marshal(pkixPublicKey{
		Algo:      pkix.AlgorithmIdentifier{Algorithm: oidPublicKeyRSA},
		BitString: asn1.BitString{Bytes: rsaPKCS1, BitLength: 8 * len(rsaPKCS1)},
	})
	// an unsupported type
	pubs = append(pubs, struct{}{})

	type parser struct {
		name     string
		got, exp func([]byte) (any, error)
	}
	parsers := []parser{
		{"PKCS8", func(b []byte) (any, error) { return x5ParsePKCS8PrivateKey(b) }, func(b []byte) (any, error) { return x509.ParsePKCS8PrivateKey(b) }},
		{"PKIX", func(b []byte) (any, error) { return x5ParsePKIXPublicKey(b) }, func(b []byte) (any, error) { return x509.ParsePKIXPublicKey(b) }},
		{"PKCS1", func(b []byte) (any, error) { return x5ParsePKCS1PrivateKey(b) }, func(b []byte) (any, error) { return x509.ParsePKCS1PrivateKey(b) }},
		{"PKCS1pub", func(b []byte) (any, error) { return x5ParsePKCS1PublicKey(b) }, func(b []byte) (any, error) { return x509.ParsePKCS1PublicKey(b) }},
		{"EC", func(b []byte) (any, error) { return x5ParseECPrivateKey(b) }, func(b []byte) (any, error) { return x509.ParseECPrivateKey(b) }},
	}
	ders := [][]byte{dsaDER, rsaNoNull}
	check := func(what string, a, b []byte, ea, eb error) {
		t.Helper()
		if hex.EncodeToString(a) != hex.EncodeToString(b) || x5errText(ea) != x5errText(eb) {
			t.Errorf("%s: port %x, %v; Go %x, %v", what, a, ea, b, eb)
		}
	}
	for _, k := range privs {
		a, ea := x5MarshalPKCS8PrivateKey(k)
		b, eb := x509.MarshalPKCS8PrivateKey(k)
		check(fmt.Sprintf("MarshalPKCS8PrivateKey(%T)", k), a, b, ea, eb)
		ders = append(ders, b)
		switch k := k.(type) {
		case *rsa.PrivateKey:
			check("MarshalPKCS1PrivateKey", x5MarshalPKCS1PrivateKey(k), x509.MarshalPKCS1PrivateKey(k), nil, nil)
			ders = append(ders, x509.MarshalPKCS1PrivateKey(k))
		case *ecdsa.PrivateKey:
			a, ea := x5MarshalECPrivateKey(k)
			b, eb := x509.MarshalECPrivateKey(k)
			check("MarshalECPrivateKey", a, b, ea, eb)
			ders = append(ders, b)
		}
	}
	for _, k := range pubs {
		a, ea := x5MarshalPKIXPublicKey(k)
		b, eb := x509.MarshalPKIXPublicKey(k)
		check(fmt.Sprintf("MarshalPKIXPublicKey(%T)", k), a, b, ea, eb)
		if eb == nil {
			ders = append(ders, b)
		}
		if k, ok := k.(*rsa.PublicKey); ok {
			check("MarshalPKCS1PublicKey", x5MarshalPKCS1PublicKey(k), x509.MarshalPKCS1PublicKey(k), nil, nil)
			ders = append(ders, x509.MarshalPKCS1PublicKey(k))
		}
	}
	n := 0
	for _, der := range ders {
		for _, m := range x5mutations(der) {
			for _, p := range parsers {
				ka, ea := p.got(m)
				kb, eb := p.exp(m)
				n++
				if x5errText(ea) != x5errText(eb) || (eb == nil && x5canon(ka) != x5canon(kb)) {
					t.Errorf("Parse%s(%x):\nport %s, %v\nGo   %s, %v", p.name, m, x5canon(ka), ea, x5canon(kb), eb)
				}
			}
		}
	}
	t.Logf("%d encodings, %d parses compared", len(ders), n)
}

// TestX5ECPrivateKeyBigD is an EC key whose d is longer than the curve's
// order, which ParseECPrivateKey refuses with the same text as Go.
func TestX5ECPrivateKeyBigD(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := x509.MarshalECPrivateKey(k)
	var e ecPrivateKey
	if _, err := asn1.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	e.PrivateKey = append(new(big.Int).Lsh(big.NewInt(1), 300).Bytes(), e.PrivateKey...)
	bad, _ := asn1.Marshal(e)
	_, ea := x5ParseECPrivateKey(bad)
	_, eb := x509.ParseECPrivateKey(bad)
	if x5errText(ea) != x5errText(eb) || eb == nil {
		t.Errorf("port %v, Go %v", ea, eb)
	}
}
