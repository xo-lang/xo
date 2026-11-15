package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
)

// std/crypto's RsaParts, PublicKey.rsa_parts, PrivateKey.rsa_parts,
// rsa_public_key and rsa_private_key against Go's rsa.PublicKey and
// rsa.PrivateKey with Validate and Precompute: rsa_test.go's
// TestKeyGeneration, TestModifiedPrivateKey and TestNotPrecomputed checks
// (Copyright 2009 The Go Authors, BSD-style license) over fixed 2048 bit
// keys and the keys with each number changed, with Go's errors.

var rsaPartsPEMs = []string{"-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDB4VbTYTHERXZ/\n9ObD76o0l4GPFoyb+5KOqnGnhlq3t7e8s09gdi6sGOGrVHbQK7FBrHt9Z0l2XY5A\npoVMq+q2LW81n7fdRaHtpkTuetISI4xkC1/Px3Akgqr4btBs1ZvA+uUiJfkB6p2i\nrnz8TzZleAuLaKezvGQOkSIiDkTnfQJR72VCPDAC+iL7QblYkDRXRp6PQKr/KD5/\nW9swaUFopFPDQFkVeAxSjVqlZtdQJtdpmrmn3EZbuEUb6i8YoUdOLhDuJ9NnCOOE\nPvbyIwl4Fv4bM8a3Wrbw1Sgj9bWQ+qG3aO/CLB+63b4SmJ/D10R6dPapchK56wfL\n1TUf90yLAgMBAAECggEAFTl8flaBEroB/38JdkC58tj1K3tt+DOBt1GMcKkCLGOV\n3zgIBAjcEiYFKnLsMNzfok9mE3PntGgIhyZTqHGXl8sAhi/OepBBallN6YeTkL3I\nhDlEi2QLcEvHuLu04H0/+E/fHq5mhKGxERql/x8S1ufd56yU01XX5EpaIL1MKMUz\nTEVCuuYaQ1ckXwSEL1bVehGXiayqkiPwa0Sqf7Cxt3doTUnrZPKJnZjOXS/ZtuM3\nZj0JreMA0iEIxto2R4iATsuj4MsXGuA31COnfDHY1nFQFmZTaV3BHWgE7ry+xd//\nJIvd/MkLQRG5+qeFn7504bdm+kKa0dR4ymulCOIycQKBgQDSTlaUlNbMG/wB04Jt\nv7uNTfKz+mYiqGKBHzDYhGiUaJJXxC043AZIZstPPsNQ0fmc0d15kasbLEf3pHFA\nqX429xxwD87x/ARBgr9PKBRbyH7aZkSdTOtXWRecdpbCh6QCTw/U6CGk0f0rHRgT\nEPZl56MJ5BHYLe4r+cIKqNyCOwKBgQDsAV1eeLxJrZoIvj1jVKRPXpHI9abEZgat\nweXU3IXu6rpgKplIjLp7m1Dc48kViIJR7inDE+bjrItxK6YQd0fAImd5EVZ5w2QF\na7lwGF4Irq3VQKHPYiai78fhm6YCcmBYarh2sQqRYp/qq192Z+hk0hLuMUZthVKZ\nGWfE8vvp8QKBgGb0CLiWII0tSfJO6/P0P8mOFjl7ZJKk4gwaC9yC/XthsfqbRBQo\nyj6ZbAAwVodzM4JMzuJbqGsWA2k9LGAeWXZpBJfrdUDLQf4cFrGWiljgTxF1rTNg\nW3mYWNG00FsM8gJbWAG/XfEFB64agRWcYTND8XG/H+k3mvgWPaLgRYrPAoGBANhr\nP/Xb/b4Kxk/yNFUcTzjBXirMqcs8TC+WuW4Dc4ZuFc8OAVZ4RqIHAZYuBD4M2oTL\nQ9yyKW7q0UxMXJKI73EPm5d4ALbUKKqX6IsUrVaE6L+/7yI/vXJ5M/B9TTS9MgCi\njNm67Bf6Ou91i6sHCiM6SUXIRe807VOb6n6+26fhAoGBAKoVJLW4AE6RUm/AnCdA\n1nf6UF5mwhQ3ZfxLkw8nHUP3X5QOJ1GcCAR8oIshARBb6Grf96vO4QFQxi9YNOjq\n10309Htd0Ejcc3usUj+MFsswG6AtkXKTE0x6gjoIQRoSX+FJqvKzeh2p35KsZeXC\nZComAH7nKEXIAUHFDCTh1yku\n-----END PRIVATE KEY-----\n", "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDaX5Xlv2rbmwqM\nVaX8kS8HHFi0h0aC5H1NeHA51bBz0Ip+G7MebEHAtfik1SY0A5XH+xXd/clHNywu\noHXuVNQ8ZvPJHAcvmLN5MpwPE2Yh7ykieNErqt1D8Zf49M6EfofUbDz2z7NZ2aY+\nkunPexN6hO2yU2D71m9bTRrLu/2Q8i2WedD3n0mfev6xLd/VjYUAtniPSxMSiULV\niVEmmVyyCBc5+a0EqMJhbblf3FtdG1LZe4R7jy38w8D3kSxBdRekFMll4ccyb5Y1\nDfeLmemr8u4Y+2pnivH22DjzmY5q4wKgYnwkK4xyHNx/4FY8/8wIDVS+dKWa+6m8\nAfgu/BS/AgMBAAECggEAFJXiKZBL4HvTGLuSNCbzifYQHqSJ7xje/wiVe747Lg/A\nnna6AeouoJ4G3HMDcO/NlNaEjzK3YLFAxvx1YYw5naAO5yxvOPtqH8i3kWxAun4z\nIH7HKRioVxlVPDDHrxHGeC3j6VhXDedIlql+xQJTKb+SMLdPcy4ZvT+dgQstshYV\nvVrZ4v1E5AIix5fnIqEoRy2+2jzO55PYoU2XjYTIq4+ifJcc5/8Chh3+W1OA6TRF\nA2YJ2Nsa1yetg9FQd0dwgj2uLUj4caoK/gBDzT+gooQCedTWcB3eZqaTeqfRGgTe\noPqwLg9TPECICiPg2XDk43E52DvQChEg96RMxdsCkQKBgQD17HHd4YUQl6FJoUvq\n8/pefkYgf46pz5zJIxifugtfdJvIsl6635ie1Nj3mtvxaZN+dkYLT0SXiFp9haW7\nICMjj8hAbWXgAg3eXMce79MNM8tFxAbvl0NjdmgePTkw8DtI/r5eyJRMB4e2MHn2\n5U2GWGqy/OKocOybUzsicG1phQKBgQDjUijMo5rENQ625YFgmKIHv8PhYrUnXCte\nYjVRvZeYSMOnnFcs22QRxGrNe6nMyp/asyOvwBm0R2OEcI8wy3dBf12BzaxQKjqS\n9rj5CF3H6RYhQK3b/HBkdnPUDVdXhNmWv1apYpKjzPbuAqlLUKmjvCUF+Vvxe+QA\nXxxYLsdWcwKBgErChV8RVtVTcymiCiUz5DHjq0fn3yPKRHx02eB8u0ZaeZ1KdDfV\nD12BkS+8eQ5rrPXt2J9Jiq2SOwgbn+VCAMynwhWr7CNsHHEzvSiveXZy/2GHFSCy\n2D6dWUuQCxnQfgVTw+WmjkaKp3wAQqOG08l6iX0+GV1hbtxNMhXnHavNAoGAQGiC\nCOF+M4O5BJdonf8BQeLS1GSMYXebDiYTZ3KppktPrIj+pkYPR314qXoVdWIgAu5o\n+R4b6lmj192wpKmxw4wb6XBmF1yvtf5UELOPJtpjG87gtAkFq2FoljelfLW2lSDn\npBy64dBsUqnMX/ud5Tek4dlydKyewIAyXsX6UwMCgYEAxQ0Z02/0vmOfN2PFkdr+\nUdVE8gJmsWypC8LFqJmlywK2yr6Yw0MntMfFakrv7mEXk6wzdLeYBHcDr4JvO/r7\nOjo/SkUjoxntGBvG2su8oeiYZqGKhV/AlYdFxPu4PoSiPGORLT0jNnGXoX6Jj1HD\nYS0GygF82QrQefewur05mKo=\n-----END PRIVATE KEY-----\n"}

func rsaPartsCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/rsa and its rsa_test.go (Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n")
	keys := []*rsa.PrivateKey{}
	for _, p := range rsaPartsPEMs {
		b, _ := pem.Decode([]byte(p))
		keys = append(keys, hpkeMust(x509.ParsePKCS8PrivateKey(b.Bytes)).(*rsa.PrivateKey))
	}
	bb := func(x *big.Int) string { return xbytes(x.Bytes()) }
	o.WriteString("\n/// (PKCS #8 DER, n, e, size, d, p, q, dp, dq, qinv, Go's PKCS #1 v1.5 SHA-256 signature of \"msg\" in hex)\nconst KEYS: List[(Bytes, Bytes, Int, Int, Bytes, Bytes, Bytes, Bytes, Bytes, Bytes, Str)] = [\n")
	for _, k := range keys {
		der := hpkeMust(x509.MarshalPKCS8PrivateKey(k))
		h := sha256.Sum256([]byte("msg"))
		sig := hpkeMust(rsa.SignPKCS1v15(nil, k, crypto.SHA256, h[:]))
		fmt.Fprintf(&o, "  (%s, %s, %d, %d, %s, %s, %s, %s, %s, %s, %s),\n", xbytes(der), bb(k.N), k.E, k.Size(), bb(k.D), bb(k.Primes[0]), bb(k.Primes[1]), bb(k.Precomputed.Dp), bb(k.Precomputed.Dq), bb(k.Precomputed.Qinv), xq(hex.EncodeToString(sig)))
	}
	o.WriteString("]\n")

	// TestModifiedPrivateKey and more: each number changed, Go's Validate
	// error (or "" when Go takes the key)
	o.WriteString("\n/// (what changed, n, e, d, primes, Go's error or \"\")\nconst CHANGED: List[(Str, Bytes, Int, Bytes, List[Bytes], Str)] = [\n")
	add := func(x *big.Int, d int64) *big.Int { return new(big.Int).Add(x, big.NewInt(d)) }
	for ki, k := range keys {
		p, q := k.Primes[0], k.Primes[1]
		type c struct {
			what   string
			n      *big.Int
			e      int
			d      *big.Int
			primes []*big.Int
		}
		cases := []c{
			{"as made", k.N, k.E, k.D, []*big.Int{p, q}},
			{"primes swapped", k.N, k.E, k.D, []*big.Int{q, p}},
			{"no primes", k.N, k.E, k.D, nil},
			{"one prime", k.N, k.E, k.D, []*big.Int{p}},
			{"three primes", k.N, k.E, k.D, []*big.Int{p, q, big.NewInt(3)}},
			{"n + 2", add(k.N, 2), k.E, k.D, []*big.Int{p, q}},
			{"n + 1", add(k.N, 1), k.E, k.D, []*big.Int{p, q}},
			{"n zero", big.NewInt(0), k.E, k.D, []*big.Int{p, q}},
			{"e + 2", k.N, k.E + 2, k.D, []*big.Int{p, q}},
			{"e 1", k.N, 1, k.D, []*big.Int{p, q}},
			{"e even", k.N, k.E + 1, k.D, []*big.Int{p, q}},
			{"e 0", k.N, 0, k.D, []*big.Int{p, q}},
			{"d + 2", k.N, k.E, add(k.D, 2), []*big.Int{p, q}},
			{"d zero", k.N, k.E, big.NewInt(0), []*big.Int{p, q}},
			{"d = n", k.N, k.E, k.N, []*big.Int{p, q}},
			{"p + 2", k.N, k.E, k.D, []*big.Int{add(p, 2), q}},
			{"q + 2", k.N, k.E, k.D, []*big.Int{p, add(q, 2)}},
			{"p even", k.N, k.E, k.D, []*big.Int{add(p, 1), q}},
			{"p 1", k.N, k.E, k.D, []*big.Int{big.NewInt(1), q}},
			{"p zero", k.N, k.E, k.D, []*big.Int{big.NewInt(0), q}},
			{"p = n", k.N, k.E, k.D, []*big.Int{k.N, q}},
		}
		if ki > 0 {
			cases = cases[:3]
		}
		for _, w := range cases {
			x := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: w.n, E: w.e}, D: w.d, Primes: w.primes}
			msg := ""
			if len(w.primes) > 2 {
				msg = "crypto/rsa: multi-prime keys are not supported"
			} else if err := x.Validate(); err != nil {
				msg = err.Error()
			}
			ps := make([]string, len(w.primes))
			for i, p := range w.primes {
				ps[i] = bb(p)
			}
			fmt.Fprintf(&o, "  (%s, %s, %d, %s, [%s], %s),\n", xq(w.what), bb(w.n), w.e, bb(w.d), strings.Join(ps, ", "), xq(msg))
		}
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (n, e, Go's PKIX DER in hex, or Go's error)\nconst PUBLICS: List[(Bytes, Int, Str)] = [\n")
	k := keys[0]
	for _, w := range []struct {
		n *big.Int
		e int
	}{{k.N, k.E}, {k.N, 3}, {big.NewInt(0), k.E}, {k.N, 0}, {k.N, -1}, {add(k.N, 1), k.E}, {big.NewInt(15), 3}} {
		out := ""
		switch {
		case w.n.Sign() == 0:
			out = "crypto/rsa: missing public modulus"
		case w.e < 1:
			out = "crypto/rsa: public exponent too small or negative"
		default:
			out = hex.EncodeToString(hpkeMust(x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: w.n, E: w.e})))
		}
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", bb(w.n), w.e, xq(out))
	}
	o.WriteString("]\n")

	o.WriteString(`
test "rsa_parts are Go's N, E, Size, D, Primes and Precomputed (TestKeyGeneration)" {
  for (der, n, e, size, d, p, q, dp, dq, qinv, sig) in KEYS {
    let k = crypto.parse_private_key(der) ?? e => fault(e.msg)
    let r = k.rsa_parts() ?? e => fault(e.msg)
    expect(r) == crypto.RsaParts{n: n, e: e, size: size, d: d, primes: [p, q], dp: dp, dq: dq, qinv: qinv}
    let pr = k.public_key().rsa_parts() ?? e => fault(e.msg)
    expect(pr) == crypto.RsaParts{n: n, e: e, size: size, d: b"", primes: [], dp: b"", dq: b"", qinv: b""}
    // the key again from its numbers, without the CRT values (TestNotPrecomputed)
    let k2 = crypto.rsa_private_key(n, e, d, [p, q]) ?? e => fault(e.msg)
    expect(k2.rsa_parts() ?? e => fault(e.msg)) == r
    expect(crypto.marshal_private_key(k2)) == der
    expect(k2.public_key()) == k.public_key()
    expect(crypto.verify(k2.public_key(), crypto.Hash.Sha256, b"msg", Bytes.from_hex(sig) ?? b"")) == true
    let s2 = crypto.sign(Zeros{}, k2, crypto.Hash.Sha256, b"msg") ?? e => fault(e.msg)
    expect(s2.to_hex()) == sig
    let pk = crypto.rsa_public_key(n, e) ?? e => fault(e.msg)
    expect(pk) == k.public_key()
    // leading zeros are no part of a number
    let k3 = crypto.rsa_private_key(b"\x00" + n, e, b"\x00\x00" + d, [b"\x00" + p, q]) ?? e => fault(e.msg)
    expect(k3.rsa_parts() ?? e => fault(e.msg)) == r
    expect(k3.public_key()) == k.public_key()
    expect(crypto.marshal_private_key(k3)) == der
    expect(crypto.rsa_public_key(b"\x00\x00" + n, e) ?? e => fault(e.msg)) == k.public_key()
  }
}

test "a changed number is Go's Validate error (TestModifiedPrivateKey)" {
  var bad: List[Str] = []
  for (what, n, e, d, primes, want) in CHANGED {
    let got = match crypto.rsa_private_key(n, e, d, primes) {
      Ok(_) => ""
      Err(x) => x.msg
    }
    if got != want {
      bad.push("${what}: ${got}, want ${want}")
    }
  }
  expect(bad) == []
}

test "rsa_public_key is Go's rsa.PublicKey" {
  var bad: List[Str] = []
  for (n, e, want) in PUBLICS {
    let got = match crypto.rsa_public_key(n, e) {
      Ok(k) => crypto.marshal_public_key(k).to_hex()
      Err(x) => x.msg
    }
    if got != want {
      bad.push("${n.len()} ${e}: ${got}")
    }
  }
  expect(bad) == []
}

test "rsa_parts of keys that are not RSA" {
  let ek = crypto.ecdsa_private_key(crypto.KeyAlg.EcdsaP256, Zeros{}.bytes(31) + b"\x01") ?? e => fault(e.msg)
  expect(ek.rsa_parts().is_ok()) == false
  expect(ek.public_key().rsa_parts().is_ok()) == false
}

struct Zeros {}

fn (s: Zeros) int(lo: Int, hi: Int) -> Int uses rand { lo }
fn (s: Zeros) float() -> Float uses rand { 0.5 }
fn (s: Zeros) bool() -> Bool uses rand { false }
fn (s: Zeros) shuffle[T](xs: List[T]) -> List[T] uses rand { xs }
fn (s: Zeros) bytes(n: Int) -> Bytes uses rand {
  var b = BytesBuilder.with_capacity(n)
  for _ in 0..n {
    b.write_byte(0)
  }
  b.freeze()
}
`)
	return o.String()
}
