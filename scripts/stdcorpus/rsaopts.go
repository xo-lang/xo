package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	mrand "math/rand"
	"strings"
)

// std/crypto's sign_pss_digest, verify_pss_digest, sign_pkcs1v15_digest,
// verify_pkcs1v15_digest and encrypt_oaep and decrypt_oaep's mgf_hash
// against Go's SignPSS and VerifyPSS with PSSOptions, SignPKCS1v15 and
// VerifyPKCS1v15 with a hash and with crypto.Hash(0), and
// EncryptOAEPWithOptions and Decrypt with OAEPOptions: pss_test.go's
// TestPSSSigning and TestInvalidPSSSaltLength, pkcs1v15_test.go's
// TestUnpaddedSignature, rsa_test.go's TestEncryptDecryptOAEP (Copyright
// 2013 The Go Authors, BSD-style license) over two 2048 bit keys. Go
// signs and encrypts with ctRand's zero bytes (x509create.go), so the
// salts and seeds are zeros and Xo, given zeros, makes the same bytes.

func rsaOptsCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/rsa and its pss_test.go, pkcs1v15_test.go and rsa_test.go (Copyright 2013 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n")
	keys := []*rsa.PrivateKey{}
	o.WriteString("\n/// the keys, PKCS #8 DER\nconst KEYS: List[Bytes] = [\n")
	for _, p := range rsaPartsPEMs {
		b, _ := pem.Decode([]byte(p))
		keys = append(keys, hpkeMust(x509.ParsePKCS8PrivateKey(b.Bytes)).(*rsa.PrivateKey))
		fmt.Fprintf(&o, "  %s,\n", xbytes(b.Bytes))
	}
	o.WriteString("]\n")
	hashes := []struct {
		enum string
		h    crypto.Hash
	}{{"Sha1", crypto.SHA1}, {"Sha256", crypto.SHA256}, {"Sha384", crypto.SHA384}, {"Sha512", crypto.SHA512}, {"Sha3_256", crypto.SHA3_256}, {"Md5", crypto.MD5}}
	o.WriteString("\nconst HASHES: List[crypto.Hash] = [")
	for i, h := range hashes {
		if i > 0 {
			o.WriteString(", ")
		}
		o.WriteString("crypto.Hash." + h.enum)
	}
	o.WriteString("]\n")
	r := mrand.New(mrand.NewSource(2013))
	rnd := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}
	res := func(b []byte, err error) string {
		if err != nil {
			return "err " + err.Error()
		}
		return hex.EncodeToString(b)
	}

	// TestPSSSigning and TestInvalidPSSSaltLength: every salt length form,
	// and Go's VerifyPSS of each signature under each form
	o.WriteString("\n/// (key, hash, digest, salt_len, Go's signature in hex or \"err ...\", Go's VerifyPSS under salt_len 0, -1, the salt's length and one more)\nconst PSS: List[(Int, Int, Bytes, Int, Str, List[Bool])] = [\n")
	for ki, k := range keys {
		for hi, h := range hashes[:5] {
			hl := h.h.Size()
			max := (k.N.BitLen()-1+7)/8 - 2 - hl
			for _, sl := range []int{-1, 0, 1, hl, max, max + 1, -2, 1000} {
				for _, dl := range []int{hl, hl - 1} {
					if dl != hl && sl != -1 {
						continue
					}
					dg := rnd(dl)
					sig, err := rsa.SignPSS(ctRand(), k, h.h, dg, &rsa.PSSOptions{SaltLength: sl})
					var vs []string
					if err == nil {
						actual := map[int]int{-1: hl, 0: max}[sl]
						if sl > 0 {
							actual = sl
						}
						for _, v := range []int{0, -1, actual, actual + 1} {
							vs = append(vs, fmt.Sprint(rsa.VerifyPSS(&k.PublicKey, h.h, dg, sig, &rsa.PSSOptions{SaltLength: v}) == nil))
						}
					}
					fmt.Fprintf(&o, "  (%d, %d, %s, %d, %s, [%s]),\n", ki, hi, xbytes(dg), sl, xq(res(sig, err)), strings.Join(vs, ", "))
				}
			}
		}
	}
	o.WriteString("]\n")

	// SignPKCS1v15 with a hash and with none (TestUnpaddedSignature)
	o.WriteString("\n/// (key, index in HASHES or -1 for none, digest, Go's signature in hex or \"err ...\")\nconst PKCS1: List[(Int, Int, Bytes, Str)] = [\n")
	for ki, k := range keys {
		for hi, h := range hashes {
			for _, dl := range []int{h.h.Size(), h.h.Size() + 1} {
				dg := rnd(dl)
				sig, err := rsa.SignPKCS1v15(nil, k, h.h, dg)
				fmt.Fprintf(&o, "  (%d, %d, %s, %s),\n", ki, hi, xbytes(dg), xq(res(sig, err)))
			}
		}
		for _, dl := range []int{0, 1, 36, 100, k.Size() - 11, k.Size() - 10, k.Size()} {
			dg := rnd(dl)
			sig, err := rsa.SignPKCS1v15(nil, k, 0, dg)
			fmt.Fprintf(&o, "  (%d, -1, %s, %s),\n", ki, xbytes(dg), xq(res(sig, err)))
		}
	}
	o.WriteString("]\n")

	// a raw signature over SHA-256's DigestInfo prefix and a digest a byte
	// too long: Go's VerifyPKCS1v15 with SHA-256 checks the length first
	prefix := hpkeHex("3031300d060960864801650304020105000420")
	o.WriteString("\n/// (key, SHA-256's DigestInfo prefix, a 33 byte digest, Go's raw signature of the two in hex)\nconst LONG: List[(Int, Bytes, Bytes, Str)] = [\n")
	for ki, k := range keys {
		dg := rnd(33)
		sig := hpkeMust(rsa.SignPKCS1v15(nil, k, 0, append(append([]byte{}, prefix...), dg...)))
		if rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA256, dg, sig) == nil {
			panic("rsa: Go verified a long digest")
		}
		fmt.Fprintf(&o, "  (%d, %s, %s, %s),\n", ki, xbytes(prefix), xbytes(dg), xq(hex.EncodeToString(sig)))
	}
	o.WriteString("]\n")

	// OAEP with MGF1 over another hash (EncryptOAEPWithOptions)
	o.WriteString("\n/// (key, hash, MGF1 hash or -1 for the hash's, label, message, Go's ciphertext in hex or \"err ...\")\nconst OAEP: List[(Int, Int, Int, Bytes, Bytes, Str)] = [\n")
	for ki, k := range keys {
		for hi, h := range hashes[:5] {
			for mi := -1; mi < 5; mi++ {
				if (mi+hi)%2 == 1 && mi != -1 {
					continue
				}
				mh := crypto.Hash(0)
				if mi >= 0 {
					mh = hashes[mi].h
				}
				label := rnd(r.Intn(3) * 7)
				ml := r.Intn(k.Size() - 2*h.h.Size() - 1)
				if mi == 2 {
					ml = k.Size() - 2*h.h.Size() - 1
				}
				msg := rnd(ml)
				c, err := rsa.EncryptOAEPWithOptions(ctRand(), &k.PublicKey, msg, &rsa.OAEPOptions{Hash: h.h, MGFHash: mh, Label: label})
				if err == nil {
					back := hpkeMust(k.Decrypt(nil, c, &rsa.OAEPOptions{Hash: h.h, MGFHash: mh, Label: label}))
					if string(back) != string(msg) {
						panic("rsa: Go's OAEP round trip")
					}
				}
				fmt.Fprintf(&o, "  (%d, %d, %d, %s, %s, %s),\n", ki, hi, mi, xbytes(label), xbytes(msg), xq(res(c, err)))
			}
		}
	}
	o.WriteString("]\n")

	o.WriteString(`
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

fn key(i: Int) -> crypto.PrivateKey { crypto.parse_private_key(KEYS[i]) ?? e => fault(e.msg) }

fn res(r: Result[Bytes, crypto.KeyErr]) -> Str {
  match r {
    Ok(b) => b.to_hex()
    Err(e) => "err ${e.msg}"
  }
}

fn cut(s: Str) -> Str { if s.len() < 60 { s } else { s[0..60] } }

/// The Go backend reads the system's randomness, not the Rand, so its
/// salts and seeds are not zeros: a result that differs from Go's still
/// passes when a second call gives other bytes (the backend is not
/// deterministic) and check accepts it.
/// Whether got, unlike Go's want, is a randomized result that check accepts (the Go backend's crypto/rsa takes no
/// caller's random source since Go 1.26). again, a second result, must differ when the randomness is 8 bytes or
/// more; with fewer (a PSS salt of 1 byte) two results coincide by chance (1 in 256 for one byte: the Go backend's
/// rsaopts corpus failed tier c36c812d6, task #277).
fn random(got: Str, want: Str, again: Str, check: fn(Bytes) -> Bool, entropy: Int) -> Bool {
  if got.starts_with("err") || want.starts_with("err") || (again == got && entropy >= 8) {
    return false
  }
  check(Bytes.from_hex(got) ?? b"")
}

fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "a short salt may repeat: random accepts the same randomized result twice below 8 bytes of entropy" {
  expect(random("ab", "cd", "ab", fn(_) { true }, 1)) == true
  expect(random("ab", "cd", "ab", fn(_) { true }, 32)) == false
  expect(random("ab", "cd", "ef", fn(_) { false }, 1)) == false
}

test "sign_pss_digest and verify_pss_digest are Go's SignPSS and VerifyPSS (TestPSSSigning, TestInvalidPSSSaltLength)" {
  var bad: List[Str] = []
  for (ki, hi, dg, sl, want, verifies) in PSS {
    let k = key(ki)
    let h = HASHES[hi]
    let name = "key ${ki} ${h:?} digest ${dg.len()} salt ${sl}"
    let got = res(crypto.sign_pss_digest(Zeros{}, k, h, dg, sl))
    let actual = if sl == -1 { h.size() } else if sl == 0 { (k.rsa_parts() ?? e => fault(e.msg)).size - 2 - h.size() } else { sl }
    if got != want && !random(got, want, res(crypto.sign_pss_digest(Zeros{}, k, h, dg, sl)), fn(b) { crypto.verify_pss_digest(k.public_key(), h, dg, b, actual) }, actual) {
      bad.push("${name}: ${cut(got)}")
      continue
    }
    if verifies.is_empty() {
      continue
    }
    let sig = Bytes.from_hex(want) ?? b""
    let forms = [0, -1, actual, actual + 1]
    for i in 0..4 {
      if crypto.verify_pss_digest(k.public_key(), h, dg, sig, forms[i]) != verifies[i] {
        bad.push("${name}: verify under ${forms[i]}")
      }
    }
  }
  expect(show(bad)) == []
}

test "sign_pkcs1v15_digest and verify_pkcs1v15_digest are Go's, with a hash and with none (TestUnpaddedSignature)" {
  var bad: List[Str] = []
  for (ki, hi, dg, want) in PKCS1 {
    let k = key(ki)
    let h: crypto.Hash? = if hi < 0 { None } else { Some(HASHES[hi]) }
    let name = "key ${ki} hash ${hi} digest ${dg.len()}"
    let got = res(crypto.sign_pkcs1v15_digest(k, h, dg))
    if got != want {
      bad.push("${name}: ${cut(got)}")
      continue
    }
    if !want.starts_with("err") {
      let sig = Bytes.from_hex(want) ?? b""
      if !crypto.verify_pkcs1v15_digest(k.public_key(), h, dg, sig) {
        bad.push("${name}: verify")
      }
      if hi >= 0 && crypto.verify_pkcs1v15_digest(k.public_key(), None, dg, sig) {
        bad.push("${name}: verified as unpadded")
      }
      if hi >= 0 && !crypto.verify(k.public_key(), HASHES[hi], b"", sig) && dg == crypto.digest(HASHES[hi], b"") {
        bad.push("${name}: verify of the message")
      }
    }
  }
  expect(show(bad)) == []
}

test "verify_pkcs1v15_digest checks the digest's length before the padding (Go's VerifyPKCS1v15)" {
  for (ki, prefix, dg, want) in LONG {
    let k = key(ki)
    let sig = crypto.sign_pkcs1v15_digest(k, None, prefix + dg) ?? e => fault(e.msg)
    expect(sig.to_hex()) == want
    expect(crypto.verify_pkcs1v15_digest(k.public_key(), None, prefix + dg, sig)) == true
    expect(crypto.verify_pkcs1v15_digest(k.public_key(), Some(crypto.Hash.Sha256), dg, sig)) == false
  }
}

test "encrypt_oaep and decrypt_oaep take MGF1's hash apart (EncryptOAEPWithOptions)" {
  var bad: List[Str] = []
  for (ki, hi, mi, label, msg, want) in OAEP {
    let k = key(ki)
    let h = HASHES[hi]
    let mh: crypto.Hash? = if mi < 0 { None } else { Some(HASHES[mi]) }
    let name = "key ${ki} ${h:?} mgf ${mi} msg ${msg.len()}"
    let got = res(crypto.encrypt_oaep(Zeros{}, k.public_key(), h, msg, label, mh))
    if got != want && !random(got, want, res(crypto.encrypt_oaep(Zeros{}, k.public_key(), h, msg, label, mh)), fn(c) { (crypto.decrypt_oaep(k, h, c, label, mh) ?? _ => b"!") == msg }, h.size()) {
      bad.push("${name}: ${cut(got)}")
      continue
    }
    if want.starts_with("err") {
      continue
    }
    let c = Bytes.from_hex(want) ?? b""
    if (crypto.decrypt_oaep(k, h, c, label, mh) ?? _ => b"!") != msg {
      bad.push("${name}: decrypt")
    }
    // another MGF1 hash: the one decryption error
    let other: crypto.Hash? = Some(if (if mi < 0 { hi } else { mi }) == 0 { HASHES[1] } else { HASHES[0] })
    if res(crypto.decrypt_oaep(k, h, c, label, other)) != "err crypto/rsa: decryption error" {
      bad.push("${name}: decrypt with another MGF1 hash")
    }
  }
  expect(show(bad)) == []
}
`)
	return o.String()
}
