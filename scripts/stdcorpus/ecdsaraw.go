package main

import (
	"bufio"
	"compress/bzip2"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"
	mrand "math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// std/crypto's ecdsa_public_key, PublicKey.bytes and PrivateKey.bytes,
// PublicKey.ecdh and PrivateKey.ecdh, ecdsa_sign_digest and
// ecdsa_verify_digest against Go's crypto/ecdsa: ecdsa_test.go's
// TestVectors (NIST's SigVer.rsp, from Go's testdata), TestParseAndBytesRoundTrip,
// TestInvalidPublicKeys, TestInvalidPrivateKeys and TestZeroHashSignature
// (Copyright 2011 The Go Authors, BSD-style license), and seeded keys and
// digests on every curve.

const ecdsaRSAKey = "-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDB4VbTYTHERXZ/\n9ObD76o0l4GPFoyb+5KOqnGnhlq3t7e8s09gdi6sGOGrVHbQK7FBrHt9Z0l2XY5A\npoVMq+q2LW81n7fdRaHtpkTuetISI4xkC1/Px3Akgqr4btBs1ZvA+uUiJfkB6p2i\nrnz8TzZleAuLaKezvGQOkSIiDkTnfQJR72VCPDAC+iL7QblYkDRXRp6PQKr/KD5/\nW9swaUFopFPDQFkVeAxSjVqlZtdQJtdpmrmn3EZbuEUb6i8YoUdOLhDuJ9NnCOOE\nPvbyIwl4Fv4bM8a3Wrbw1Sgj9bWQ+qG3aO/CLB+63b4SmJ/D10R6dPapchK56wfL\n1TUf90yLAgMBAAECggEAFTl8flaBEroB/38JdkC58tj1K3tt+DOBt1GMcKkCLGOV\n3zgIBAjcEiYFKnLsMNzfok9mE3PntGgIhyZTqHGXl8sAhi/OepBBallN6YeTkL3I\nhDlEi2QLcEvHuLu04H0/+E/fHq5mhKGxERql/x8S1ufd56yU01XX5EpaIL1MKMUz\nTEVCuuYaQ1ckXwSEL1bVehGXiayqkiPwa0Sqf7Cxt3doTUnrZPKJnZjOXS/ZtuM3\nZj0JreMA0iEIxto2R4iATsuj4MsXGuA31COnfDHY1nFQFmZTaV3BHWgE7ry+xd//\nJIvd/MkLQRG5+qeFn7504bdm+kKa0dR4ymulCOIycQKBgQDSTlaUlNbMG/wB04Jt\nv7uNTfKz+mYiqGKBHzDYhGiUaJJXxC043AZIZstPPsNQ0fmc0d15kasbLEf3pHFA\nqX429xxwD87x/ARBgr9PKBRbyH7aZkSdTOtXWRecdpbCh6QCTw/U6CGk0f0rHRgT\nEPZl56MJ5BHYLe4r+cIKqNyCOwKBgQDsAV1eeLxJrZoIvj1jVKRPXpHI9abEZgat\nweXU3IXu6rpgKplIjLp7m1Dc48kViIJR7inDE+bjrItxK6YQd0fAImd5EVZ5w2QF\na7lwGF4Irq3VQKHPYiai78fhm6YCcmBYarh2sQqRYp/qq192Z+hk0hLuMUZthVKZ\nGWfE8vvp8QKBgGb0CLiWII0tSfJO6/P0P8mOFjl7ZJKk4gwaC9yC/XthsfqbRBQo\nyj6ZbAAwVodzM4JMzuJbqGsWA2k9LGAeWXZpBJfrdUDLQf4cFrGWiljgTxF1rTNg\nW3mYWNG00FsM8gJbWAG/XfEFB64agRWcYTND8XG/H+k3mvgWPaLgRYrPAoGBANhr\nP/Xb/b4Kxk/yNFUcTzjBXirMqcs8TC+WuW4Dc4ZuFc8OAVZ4RqIHAZYuBD4M2oTL\nQ9yyKW7q0UxMXJKI73EPm5d4ALbUKKqX6IsUrVaE6L+/7yI/vXJ5M/B9TTS9MgCi\njNm67Bf6Ou91i6sHCiM6SUXIRe807VOb6n6+26fhAoGBAKoVJLW4AE6RUm/AnCdA\n1nf6UF5mwhQ3ZfxLkw8nHUP3X5QOJ1GcCAR8oIshARBb6Grf96vO4QFQxi9YNOjq\n10309Htd0Ejcc3usUj+MFsswG6AtkXKTE0x6gjoIQRoSX+FJqvKzeh2p35KsZeXC\nZComAH7nKEXIAUHFDCTh1yku\n-----END PRIVATE KEY-----\n"

var ecdsaCurves = []struct {
	enum  string
	curve elliptic.Curve
}{{"EcdsaP224", elliptic.P224()}, {"EcdsaP256", elliptic.P256()}, {"EcdsaP384", elliptic.P384()}, {"EcdsaP521", elliptic.P521()}}

func ecdsaDER(r, s *big.Int) []byte {
	return hpkeMust(asn1.Marshal(struct{ R, S *big.Int }{r, s}))
}

func ecdsaRawCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/ecdsa, its ecdsa_test.go and testdata/SigVer.rsp.bz2 (NIST CAVP; Copyright 2011 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n\nconst ALGS: List[crypto.KeyAlg] = [")
	for i, c := range ecdsaCurves {
		if i > 0 {
			o.WriteString(", ")
		}
		o.WriteString("crypto.KeyAlg." + c.enum)
	}
	o.WriteString("]\n")
	fmt.Fprintf(&o, "\nconst RSA_KEY: Str = %s\n", xq(ecdsaRSAKey))
	size := func(c elliptic.Curve) int { return (c.Params().BitSize + 7) / 8 }
	point := func(c elliptic.Curve, x, y *big.Int) []byte {
		n := size(c)
		b := make([]byte, 1+2*n)
		b[0] = 4
		x.FillBytes(b[1 : 1+n])
		y.FillBytes(b[1+n:])
		return b
	}

	// TestVectors
	f := hpkeMust(os.Open(filepath.Join(runtime.GOROOT(), "src/crypto/ecdsa/testdata/SigVer.rsp.bz2")))
	defer f.Close()
	sc := bufio.NewScanner(bzip2.NewReader(f))
	o.WriteString("\n/// (index in ALGS, the public point, the digest, the DER signature, NIST's and Go's verdict, ecdsa_public_key's error or \"\")\nconst VECTORS: List[(Int, Bytes, Bytes, Bytes, Bool, Str)] = [\n")
	ci, hi := -1, hash.Hash(nil)
	var msg []byte
	var qx, qy, r, s *big.Int
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			parts := strings.Split(strings.Trim(line, "[]"), ",")
			ci = map[string]int{"P-224": 0, "P-256": 1, "P-384": 2, "P-521": 3}[parts[0]]
			switch parts[1] {
			case "SHA-1":
				hi = sha1.New()
			case "SHA-224":
				hi = sha256.New224()
			case "SHA-256":
				hi = sha256.New()
			case "SHA-384":
				hi = sha512.New384()
			case "SHA-512":
				hi = sha512.New()
			}
			continue
		}
		k, v, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		bi := func() *big.Int { x, _ := new(big.Int).SetString(v, 16); return x }
		switch k {
		case "Msg":
			msg = hpkeHex(v)
		case "Qx":
			qx = bi()
		case "Qy":
			qy = bi()
		case "R":
			r = bi()
		case "S":
			s = bi()
		case "Result":
			c := ecdsaCurves[ci].curve
			hi.Reset()
			hi.Write(msg)
			dg := hi.Sum(nil)
			pt := point(c, qx, qy)
			sig := ecdsaDER(r, s)
			want := v[0] == 'P'
			perr := ""
			pk, err := ecdsa.ParseUncompressedPublicKey(c, pt)
			if err != nil {
				perr = err.Error()
			}
			got := err == nil && ecdsa.VerifyASN1(pk, dg, sig)
			if got != want {
				panic("ecdsa: Go disagrees with NIST")
			}
			fmt.Fprintf(&o, "  (%d, %s, %s, %s, %v, %s),\n", ci, xbytes(pt), xbytes(dg), xbytes(sig), want, xq(perr))
		}
	}
	o.WriteString("]\n")

	// Wycheproof's P-256 SHA-256 verify vectors, each checked against Go
	o.WriteString("\n/// Wycheproof's ecdsa_secp256r1_sha256_test.json (Apache 2.0, Copyright Google LLC and the Wycheproof authors): (tcId, the public point, the digest, the DER signature, the verdict, Go's VerifyASN1 agreeing)\nconst WYCHEPROOF: List[(Int, Bytes, Bytes, Bytes, Bool)] = [\n")
	for _, g := range wycheproof("ecdsa_secp256r1_sha256_test.json").TestGroups {
		pt := unhex(g.PublicKey.Uncompressed)
		pk, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pt)
		if err != nil {
			panic("ecdsa: Go refuses a Wycheproof key: " + err.Error())
		}
		for _, t := range g.Tests {
			sum := sha256.Sum256(unhex(t.Msg))
			sig := unhex(t.Sig)
			want := t.Result == "valid"
			if t.Result == "acceptable" || ecdsa.VerifyASN1(pk, sum[:], sig) != want {
				panic(fmt.Sprintf("ecdsa: Go disagrees with Wycheproof tcId %d", t.TcID))
			}
			fmt.Fprintf(&o, "  (%d, %s, %s, %s, %v),\n", t.TcID, xbytes(pt), xbytes(sum[:]), xbytes(sig), want)
		}
	}
	o.WriteString("]\n")

	// TestParseAndBytesRoundTrip over seeded scalars, with Go's ECDH secret
	// of two keys
	rr := mrand.New(mrand.NewSource(2011))
	o.WriteString("\n/// (index in ALGS, d, Go's public point, a peer's d, Go's ECDH secret in hex or \"\" for P-224)\nconst KEYS: List[(Int, Bytes, Bytes, Bytes, Str)] = [\n")
	scalar := func(c elliptic.Curve) []byte {
		for {
			b := make([]byte, size(c))
			rr.Read(b)
			if c == elliptic.P521() {
				b[0] &= 1
			}
			if k, err := ecdsa.ParseRawPrivateKey(c, b); err == nil {
				_ = k
				return b
			}
		}
	}
	for ci, cc := range ecdsaCurves {
		for i := 0; i < 6; i++ {
			d, peer := scalar(cc.curve), scalar(cc.curve)
			k := hpkeMust(ecdsa.ParseRawPrivateKey(cc.curve, d))
			pub := hpkeMust(k.PublicKey.Bytes())
			if !bytesEq(hpkeMust(k.Bytes()), d) {
				panic("ecdsa: Bytes")
			}
			secret := ""
			if cc.curve != elliptic.P224() {
				e := hpkeMust(k.ECDH())
				p := hpkeMust(hpkeMust(ecdsa.ParseRawPrivateKey(cc.curve, peer)).PublicKey.ECDH())
				secret = hex.EncodeToString(hpkeMust(e.ECDH(p)))
				if !bytesEq(e.PublicKey().Bytes(), pub) {
					panic("ecdsa: ECDH public")
				}
			} else if _, err := k.ECDH(); err == nil || err.Error() != "ecdsa: unsupported curve by crypto/ecdh" {
				panic("ecdsa: P-224 ECDH")
			}
			fmt.Fprintf(&o, "  (%d, %s, %s, %s, %s),\n", ci, xbytes(d), xbytes(pub), xbytes(peer), xq(secret))
		}
	}
	o.WriteString("]\n")

	// TestInvalidPublicKeys, and the other forms Go refuses
	o.WriteString("\n/// (index in ALGS, a point Go refuses, Go's error)\nconst BAD_POINTS: List[(Int, Bytes, Str)] = [\n")
	for ci, cc := range ecdsaCurves {
		c := cc.curve
		n := size(c)
		k := hpkeMust(ecdsa.ParseRawPrivateKey(c, scalar(c)))
		good := hpkeMust(k.PublicKey.Bytes())
		notOn := append([]byte(nil), good...)
		notOn[n] ^= 1
		bigX := point(c, c.Params().P, k.PublicKey.Y)
		yp := new(big.Int).Add(k.PublicKey.Y, c.Params().P)
		if yp.BitLen() > 8*n {
			yp = c.Params().P
		}
		bigY := point(c, k.PublicKey.X, yp)
		zeros := make([]byte, 1+2*n)
		zeros[0] = 4
		for _, b := range [][]byte{{0}, nil, zeros, notOn, elliptic.MarshalCompressed(c, k.PublicKey.X, k.PublicKey.Y), good[:len(good)-1], append(append([]byte(nil), good...), 0), bigX, bigY, {4}} {
			_, err := ecdsa.ParseUncompressedPublicKey(c, b)
			if err == nil {
				panic("ecdsa: Go took a bad point")
			}
			fmt.Fprintf(&o, "  (%d, %s, %s),\n", ci, xbytes(b), xq(err.Error()))
		}
	}
	o.WriteString("]\n")

	// SignASN1 digests: Go's signatures over digests of every length
	// (TestZeroHashSignature's 64 zero bytes among them)
	o.WriteString("\n/// (index in ALGS, d, a digest, a signature Go made of it)\nconst DIGESTS: List[(Int, Bytes, Bytes, Bytes)] = [\n")
	for ci, cc := range ecdsaCurves {
		d := scalar(cc.curve)
		k := hpkeMust(ecdsa.ParseRawPrivateKey(cc.curve, d))
		for _, n := range []int{0, 1, 20, 28, 32, 48, 64, 66, 100} {
			dg := make([]byte, n)
			if n != 64 {
				rr.Read(dg)
			}
			sig := hpkeMust(ecdsa.SignASN1(ctRand(), k, dg))
			if !ecdsa.VerifyASN1(&k.PublicKey, dg, sig) {
				panic("ecdsa: Go refuses its own signature")
			}
			fmt.Fprintf(&o, "  (%d, %s, %s, %s),\n", ci, xbytes(d), xbytes(dg), xbytes(sig))
		}
	}
	o.WriteString("]\n")

	o.WriteString(`
/// A Rand that gives zeros: the hedged nonce still takes the key and the
/// digest.
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

fn flip(b: Bytes, i: Int) -> Bytes {
  var w = BytesBuilder.with_capacity(b.len())
  w.write(b)
  w.put_le(i, Int(b[i]) ^ 1, 1)
  w.freeze()
}

fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "ecdsa_verify_digest is Go's VerifyASN1 over NIST's SigVer vectors (TestVectors)" {
  var bad: List[Str] = []
  var n = 0
  for (ci, pt, dg, sig, want, perr) in VECTORS {
    n += 1
    match crypto.ecdsa_public_key(ALGS[ci], pt) {
      Ok(k) => {
        if perr != "" {
          bad.push("${n}: parsed, Go: ${perr}")
        } else if crypto.ecdsa_verify_digest(k, dg, sig) != want {
          bad.push("${n}: verify")
        }
      }
      Err(e) => if e.msg != perr || want {
        bad.push("${n}: ${e.msg}, Go: ${perr}")
      }
    }
  }
  expect(show(bad)) == []
}

test "ecdsa_verify_digest is Go's VerifyASN1 over Wycheproof's P-256 SHA-256 vectors" {
  var bad: List[Str] = []
  for (id, pt, dg, sig, want) in WYCHEPROOF {
    let k = crypto.ecdsa_public_key(crypto.KeyAlg.EcdsaP256, pt) ?? e => fault(e.msg)
    if crypto.ecdsa_verify_digest(k, dg, sig) != want {
      bad.push("tcId ${id}")
    }
  }
  expect(show(bad)) == []
}

test "the raw encodings and ECDH round trip (TestParseAndBytesRoundTrip)" {
  var bad: List[Str] = []
  for (ci, d, point, peer, secret) in KEYS {
    let name = "${ALGS[ci]:?}"
    let k = crypto.ecdsa_private_key(ALGS[ci], d) ?? e => fault(e.msg)
    if (k.bytes() ?? _ => b"") != d || (k.public_key().bytes() ?? _ => b"") != point {
      bad.push("${name}: bytes")
    }
    let p = crypto.ecdsa_public_key(ALGS[ci], point) ?? e => fault(e.msg)
    if p != k.public_key() || crypto.marshal_public_key(p) != crypto.marshal_public_key(k.public_key()) {
      bad.push("${name}: parsed point")
    }
    match (k.ecdh(), p.ecdh()) {
      (Ok(e), Ok(ep)) => {
        let pk = crypto.ecdsa_private_key(ALGS[ci], peer) ?? e => fault(e.msg)
        let q = pk.public_key().ecdh() ?? e => fault(e.msg)
        if (e.ecdh(q) ?? _ => b"").to_hex() != secret {
          bad.push("${name}: ECDH secret")
        }
        if e.bytes() != d || e.public_key() != ep || ep.bytes() != point {
          bad.push("${name}: ECDH keys")
        }
        let back = crypto.ecdh_private_key(e.curve(), d) ?? e => fault(e.msg)
        if back.public_key() != ep {
          bad.push("${name}: ecdh_private_key")
        }
      }
      (Err(a), Err(b)) => if secret != "" || a.msg != "ecdsa: unsupported curve by crypto/ecdh" || b.msg != a.msg {
        bad.push("${name}: ${a.msg}")
      }
      _ => bad.push("${name}: one ecdh")
    }
  }
  expect(bad) == []
}

test "the points Go refuses, with Go's errors (TestInvalidPublicKeys)" {
  var bad: List[Str] = []
  for (ci, pt, want) in BAD_POINTS {
    match crypto.ecdsa_public_key(ALGS[ci], pt) {
      Ok(_) => bad.push("${ALGS[ci]:?} ${pt.len()}: parsed")
      Err(e) => if e.msg != want {
        bad.push("${ALGS[ci]:?} ${pt.len()}: ${e.msg}, want ${want}")
      }
    }
  }
  let p256 = KEYS[6].2
  for alg in [crypto.KeyAlg.Rsa, crypto.KeyAlg.Ed25519] {
    match crypto.ecdsa_public_key(alg, p256) {
      Ok(_) => bad.push("${alg:?}: parsed")
      Err(e) => if e.msg != "ecdsa: curve not supported by ParseUncompressedPublicKey" {
        bad.push("${alg:?}: ${e.msg}")
      }
    }
  }
  expect(bad) == []
}

test "ecdsa_sign_digest and ecdsa_verify_digest are Go's SignASN1 and VerifyASN1 (TestZeroHashSignature)" {
  var bad: List[Str] = []
  let r = Zeros{}
  for (ci, d, dg, gosig) in DIGESTS {
    let name = "${ALGS[ci]:?} digest ${dg.len()}"
    let k = crypto.ecdsa_private_key(ALGS[ci], d) ?? e => fault(e.msg)
    let p = k.public_key()
    if !crypto.ecdsa_verify_digest(p, dg, gosig) {
      bad.push("${name}: Go's signature")
    }
    let sig = crypto.ecdsa_sign_digest(r, k, dg) ?? e => fault(e.msg)
    if !crypto.ecdsa_verify_digest(p, dg, sig) {
      bad.push("${name}: own signature")
    }
    if crypto.ecdsa_verify_digest(p, dg + b"x", sig) && dg.len() < [28, 32, 48, 66][ci] || crypto.ecdsa_verify_digest(p, dg, flip(sig, sig.len() - 1)) || crypto.ecdsa_verify_digest(p, dg, sig[0..sig.len() - 1]) {
      bad.push("${name}: a changed digest or signature")
    }
    if dg.len() == 32 && ci == 1 && !crypto.verify(p, crypto.Hash.Sha256, b"", crypto.ecdsa_sign_digest(r, k, crypto.sha256(b"")) ?? _ => b"") {
      bad.push("${name}: verify of a sign_digest")
    }
  }
  expect(bad) == []
}

test "bytes and ecdh of RSA and Ed25519 keys" {
  let blocks = crypto.pem_decode(RSA_KEY)
  let rk = crypto.parse_private_key(blocks[0].bytes) ?? e => fault(e.msg)
  expect(rk.bytes().is_ok()) == false
  expect(rk.public_key().bytes().is_ok()) == false
  expect(rk.ecdh().is_ok()) == false
  expect(rk.public_key().ecdh().is_ok()) == false
  expect(crypto.ecdsa_sign_digest(Zeros{}, rk, b"x").is_ok()) == false
  expect(crypto.ecdsa_verify_digest(rk.public_key(), b"x", b"x")) == false
  let seed = Zeros{}.bytes(32)
  let ek = crypto.generate_key(Zeros{}, crypto.KeyAlg.Ed25519) ?? e => fault(e.msg)
  expect(ek.bytes() ?? _ => b"") == seed
  expect(ek.public_key().bytes() ?? _ => b"") == crypto.ed25519_public_key(seed)
  expect(ek.ecdh().is_ok()) == false
}
`)
	return o.String()
}

func bytesEq(a, b []byte) bool { return string(a) == string(b) }
