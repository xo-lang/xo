package main

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	mrand "math/rand"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// std/x509's key encodings against Go's crypto/x509: ParsePKCS1PrivateKey,
// ParsePKCS1PublicKey, ParseECPrivateKey, ParsePKCS8PrivateKey and
// ParsePKIXPublicKey of pkcs8_test.go's and sec1_test.go's keys (Copyright
// 2011 and 2012 The Go Authors, BSD-style license), of each key's other
// encodings and of random mutations of them; and MarshalPKCS1PrivateKey,
// MarshalPKCS1PublicKey and MarshalECPrivateKey of every key that parses.

// keysGoFixtures is every hex DER string literal of pkcs8_test.go and
// sec1_test.go (the keys of their tables and constants).
func keysGoFixtures() [][]byte {
	var out [][]byte
	seen := map[string]bool{}
	for _, name := range []string{"pkcs8_test.go", "sec1_test.go"} {
		path := filepath.Join(runtime.GOROOT(), "src", "crypto", "x509", name)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			panic(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil || len(s) < 40 {
				return true
			}
			b, err := hex.DecodeString(s)
			if err != nil || b[0] != 0x30 || seen[s] {
				return true
			}
			seen[s] = true
			out = append(out, b)
			return true
		})
	}
	if len(out) < 10 {
		panic("too few key fixtures")
	}
	return out
}

// keysMalformed maps Go's encoding/asn1 errors as std/crypto does (the
// Go backend's Crypto_parse_private_key): a bare asn1 error is the
// function's malformed text, one inside another error "malformed DER".
func keysMalformed(m, bare string) string {
	if strings.HasPrefix(m, "asn1:") {
		return bare
	}
	if i := strings.Index(m, "asn1:"); i >= 0 {
		return m[:i] + "malformed DER"
	}
	return m
}

const (
	keysBadPriv = "x509: malformed private key"
	keysBadPub  = "x509: malformed public key"
	keysBadEC   = "x509: failed to parse EC private key: malformed DER"
	keysX25519  = "x509: PKCS#8 wrapping contained private key with unknown algorithm: 1.3.101.110"
	keysMulti   = "crypto/rsa: multi-prime keys are not supported"
)

// keysPriv is "ok PKCS8HEX" for a key Go parsed, else "err TEXT".
func keysPriv(k any, err error, bare string) string {
	if err != nil {
		return "err " + keysMalformed(err.Error(), bare)
	}
	switch x := k.(type) {
	case *ecdh.PrivateKey:
		// std/crypto has no X25519 private key (a deviation std/changes
		// records)
		return "err " + keysX25519
	case *rsa.PrivateKey:
		if len(x.Primes) > 2 {
			return "err " + keysMulti
		}
	}
	d, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		panic(err)
	}
	return "ok " + hex.EncodeToString(d)
}

func keysPub(k any, err error) string {
	if err != nil {
		return "err " + keysMalformed(err.Error(), keysBadPub)
	}
	switch k.(type) {
	case *ecdh.PublicKey:
		return "err x509: unknown public key algorithm"
	}
	d, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		return "err x509: unknown public key algorithm"
	}
	return "ok " + hex.EncodeToString(d)
}

// keysParse is Go's five answers for der.
func keysParse(der []byte) [5]string {
	var a [5]string
	k1, e1 := x509.ParsePKCS1PrivateKey(der)
	if e1 != nil {
		a[0] = keysPriv(nil, e1, keysBadPriv)
	} else {
		a[0] = keysPriv(k1, nil, keysBadPriv)
	}
	k2, e2 := x509.ParseECPrivateKey(der)
	if e2 != nil {
		a[1] = keysPriv(nil, e2, keysBadEC)
	} else {
		a[1] = keysPriv(k2, nil, keysBadEC)
	}
	k3, e3 := x509.ParsePKCS8PrivateKey(der)
	a[2] = keysPriv(k3, e3, keysBadPriv)
	k4, e4 := x509.ParsePKCS1PublicKey(der)
	if e4 != nil {
		a[3] = keysPub(nil, e4)
	} else {
		a[3] = keysPub(k4, nil)
	}
	k5, e5 := x509.ParsePKIXPublicKey(der)
	a[4] = keysPub(k5, e5)
	return a
}

// keysOthers is k's other encodings: PKCS #8, PKCS #1 or SEC 1, PKIX and
// PKCS #1 public.
func keysOthers(k any) [][]byte {
	var out [][]byte
	if d, err := x509.MarshalPKCS8PrivateKey(k); err == nil {
		out = append(out, d)
	}
	switch x := k.(type) {
	case *rsa.PrivateKey:
		out = append(out, x509.MarshalPKCS1PrivateKey(x), x509.MarshalPKCS1PublicKey(&x.PublicKey))
		if d, err := x509.MarshalPKIXPublicKey(&x.PublicKey); err == nil {
			out = append(out, d)
		}
	case *ecdsa.PrivateKey:
		if d, err := x509.MarshalECPrivateKey(x); err == nil {
			out = append(out, d)
		}
		if d, err := x509.MarshalPKIXPublicKey(&x.PublicKey); err == nil {
			out = append(out, d)
		}
	}
	return out
}

func keysAnyPriv(der []byte) any {
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return k
	}
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k
	}
	if k, err := x509.ParseECPrivateKey(der); err == nil {
		return k
	}
	return nil
}

// keysMutate changes one thing about b: a bit, a cut, a trailing byte, a
// tag or a length byte.
func keysMutate(r *mrand.Rand, b []byte) []byte {
	b = append([]byte(nil), b...)
	switch r.Intn(6) {
	case 0:
		b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
	case 1:
		b = b[:r.Intn(len(b))]
	case 2:
		b = append(b, byte(r.Intn(256)))
	case 3:
		// a tag byte: find one of the first few elements
		i := r.Intn(min(len(b), 24))
		b[i] = []byte{0x02, 0x04, 0x30, 0x05, 0x06, 0xa0, 0xa1, 0x03}[r.Intn(8)]
	case 4:
		i := 1 + r.Intn(min(len(b)-1, 24))
		b[i]++
	default:
		// an inner INTEGER made non-minimal or negative
		for i := 0; i+2 < len(b); i++ {
			if b[i] == 0x02 && b[i+1] > 0 && b[i+1] < 0x80 && r.Intn(4) == 0 {
				if r.Intn(2) == 0 {
					b[i+2] = 0x00
				} else {
					b[i+2] |= 0x80
				}
				break
			}
		}
	}
	return b
}

func x509KeysCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/x509 key encodings against std/x509's (pkcs8_test.go's and sec1_test.go's keys are Copyright 2011 and 2012 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\nuse std/x509\n")
	r := mrand.New(mrand.NewSource(9))

	base := keysGoFixtures()
	var inputs [][]byte
	seen := map[string]bool{}
	add := func(b []byte) {
		if !seen[string(b)] {
			seen[string(b)] = true
			inputs = append(inputs, b)
		}
	}
	add([]byte{})
	add([]byte{0x30, 0x00})
	add([]byte{0x05, 0x00})
	var keys []any
	for _, b := range base {
		add(b)
		if k := keysAnyPriv(b); k != nil {
			if strings.HasPrefix(keysPriv(k, nil, ""), "ok ") {
				// a key std/crypto holds (not X25519 or multi-prime)
				keys = append(keys, k)
			}
			for _, d := range keysOthers(k) {
				add(d)
			}
		}
	}
	valid := append([][]byte(nil), inputs...)
	for i := 0; i < 400; i++ {
		add(keysMutate(r, valid[3+r.Intn(len(valid)-3)]))
	}

	o.WriteString("\n/// (DER; Go's ParsePKCS1PrivateKey, ParseECPrivateKey, ParsePKCS8PrivateKey,\n/// ParsePKCS1PublicKey, ParsePKIXPublicKey: \"ok HEX\" (the key's PKCS #8\n/// or PKIX DER) or \"err TEXT\")\nfn parse_rows() -> List[(Bytes, Str, Str, Str, Str, Str)] {\n  [\n")
	for _, b := range inputs {
		a := keysParse(b)
		fmt.Fprintf(&o, "    (%s, %s, %s, %s, %s, %s),\n", xbytes(b), xq(a[0]), xq(a[1]), xq(a[2]), xq(a[3]), xq(a[4]))
	}
	o.WriteString("  ]\n}\n")

	o.WriteString("\n/// (the key's PKCS #8 DER; Go's MarshalPKCS1PrivateKey, MarshalECPrivateKey,\n/// MarshalPKCS1PublicKey: \"ok HEX\" or \"err TEXT\"; Go's types refuse\n/// the other algorithms, std/x509's texts)\nconst MARSHAL_ROWS: List[(Bytes, Str, Str, Str)] = [\n")
	for _, k := range keys {
		p8, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			continue
		}
		p1, ec, pub := "err x509: not an RSA key", "err x509: not an ECDSA key", "err x509: not an RSA key"
		switch x := k.(type) {
		case *rsa.PrivateKey:
			p1 = "ok " + hex.EncodeToString(x509.MarshalPKCS1PrivateKey(x))
			pub = "ok " + hex.EncodeToString(x509.MarshalPKCS1PublicKey(&x.PublicKey))
		case *ecdsa.PrivateKey:
			d, err := x509.MarshalECPrivateKey(x)
			if err != nil {
				ec = "err " + err.Error()
			} else {
				ec = "ok " + hex.EncodeToString(d)
			}
		}
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xbytes(p8), xq(p1), xq(ec), xq(pub))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn priv_shown(r: Result[crypto.PrivateKey, x509.CertErr]) -> Str {
  match r {
    Ok(k) => "ok ${crypto.marshal_private_key(k).to_hex()}"
    Err(e) => "err ${e.display()}"
  }
}

fn pub_shown(r: Result[crypto.PublicKey, x509.CertErr]) -> Str {
  match r {
    Ok(k) => "ok ${crypto.marshal_public_key(k).to_hex()}"
    Err(e) => "err ${e.display()}"
  }
}

fn bytes_shown(r: Result[Bytes, x509.CertErr]) -> Str {
  match r {
    Ok(b) => "ok ${b.to_hex()}"
    Err(e) => "err ${e.display()}"
  }
}

fn report(bad: List[Str]) -> Str {
  "${bad.len()} differ:\n${bad[..if bad.len() < 12 { bad.len() } else { 12 }].join("\n")}"
}

test "parse_pkcs1_private_key, parse_ec_private_key, parse_pkcs8_private_key, parse_pkcs1_public_key and parse_pkix_public_key are Go's" {
  var bad: List[Str] = []
  for (der, p1, ec, p8, pub1, pkix) in parse_rows() {
    let got = [
      priv_shown(x509.parse_pkcs1_private_key(der)),
      priv_shown(x509.parse_ec_private_key(der)),
      priv_shown(x509.parse_pkcs8_private_key(der)),
      pub_shown(x509.parse_pkcs1_public_key(der)),
      pub_shown(x509.parse_pkix_public_key(der)),
    ]
    let want = [p1, ec, p8, pub1, pkix]
    let names = ["pkcs1", "ec", "pkcs8", "pkcs1 public", "pkix"]
    for i in 0..5 {
      if got[i] != want[i] {
        bad.push("${der.to_hex()} ${names[i]}:\n  got  ${got[i]}\n  want ${want[i]}")
      }
    }
  }
  expect(report(bad)) == "0 differ:\n"
}

test "marshal_pkcs1_private_key, marshal_ec_private_key and marshal_pkcs1_public_key are Go's" {
  var bad: List[Str] = []
  for (p8, p1, ec, pub1) in MARSHAL_ROWS {
    let k = crypto.parse_private_key(p8).ok() ?? fault("key")
    let got = [bytes_shown(x509.marshal_pkcs1_private_key(k)), bytes_shown(x509.marshal_ec_private_key(k)), bytes_shown(x509.marshal_pkcs1_public_key(k.public_key()))]
    let want = [p1, ec, pub1]
    for i in 0..3 {
      if got[i] != want[i] {
        bad.push("${p8.to_hex()} ${i}:\n  got  ${got[i]}\n  want ${want[i]}")
      }
    }
  }
  expect(report(bad)) == "0 differ:\n"
}

test "a marshalled key parses back to itself" {
  for (p8, _, _, _) in MARSHAL_ROWS {
    let k = crypto.parse_private_key(p8).ok() ?? fault("key")
    match x509.marshal_pkcs1_private_key(k) {
      Ok(d) => {
        let back = x509.parse_pkcs1_private_key(d).ok() ?? fault("pkcs1")
        expect(crypto.marshal_private_key(back)) == p8
        let pd = x509.marshal_pkcs1_public_key(k.public_key()).ok() ?? fault("public")
        let pb = x509.parse_pkcs1_public_key(pd).ok() ?? fault("public back")
        expect(pb == k.public_key()) == true
      }
      Err(_) => ()
    }
    match x509.marshal_ec_private_key(k) {
      Ok(d) => {
        let back = x509.parse_ec_private_key(d).ok() ?? fault("ec")
        expect(crypto.marshal_private_key(back)) == p8
      }
      Err(_) => ()
    }
  }
}
`)
	return o.String()
}
