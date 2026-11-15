package main

import (
	gocrypto "crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
)

// std/crypto's ed25519_sign_with and ed25519_verify_with against Go's
// PrivateKey.Sign with ed25519.Options and VerifyWithOptions:
// ed25519_test.go's TestSignVerifyHashed (RFC 8032 7.3),
// TestSignVerifyContext (RFC 8032 7.2) and TestMalleability (Copyright
// 2016 The Go Authors, BSD-style license), and seeded keys, messages and
// contexts over Ed25519, Ed25519ctx and Ed25519ph, with Go's errors.

func ed25519OptsCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/ed25519 and its ed25519_test.go (Copyright 2016 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n")

	type row struct {
		key, msg, ctx []byte
		ph            bool
	}
	var rows []row
	h := func(s string) []byte { return hpkeHex(s) }
	ph := sha512.Sum512([]byte("abc"))
	// RFC 8032 7.3 (TestSignVerifyHashed), then Go's "123" context case
	rows = append(rows, row{h("833fe62409237b9d62ec77587520911e9a759cec1d19755b7da901b96dca3d42ec172b93ad5e563bf4932c70e1245034c35467ef2efd4d64ebf819683467e2bf"), ph[:], nil, true})
	rows = append(rows, row{h("833fe62409237b9d62ec77587520911e9a759cec1d19755b7da901b96dca3d42"), ph[:], []byte("123"), true})
	// RFC 8032 7.2 (TestSignVerifyContext)
	rows = append(rows, row{h("0305334e381af78f141cb666f6199f57bc3495335a256a95bd2a55bf546663f6dfc9425e4f968f7f0c29f0259cf5f9aed6851c2bb4ad8bfb860cfee0ab248292"), h("f726936d19c800494e3fdaff20b276a8"), []byte("foo"), false})
	r := mrand.New(mrand.NewSource(8032))
	rnd := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}
	for _, cl := range []int{0, 1, 3, 32, 254, 255} {
		for _, ml := range []int{0, 1, 33, 200} {
			for _, p := range []bool{false, true} {
				seed := rnd(32)
				key := seed
				if ml == 33 {
					key = ed25519.NewKeyFromSeed(seed)
				}
				msg := rnd(ml)
				if p {
					s := sha512.Sum512(msg)
					msg = s[:]
				}
				rows = append(rows, row{key, msg, rnd(cl), p})
			}
		}
	}
	priv := func(key []byte) ed25519.PrivateKey {
		if len(key) == 64 {
			return ed25519.PrivateKey(key)
		}
		return ed25519.NewKeyFromSeed(key)
	}
	opts := func(ph bool, ctx []byte) *ed25519.Options {
		o := &ed25519.Options{Context: string(ctx)}
		if ph {
			o.Hash = gocrypto.SHA512
		}
		return o
	}
	o.WriteString("\n/// (key, message or its SHA-512, Ed25519ph, context, Go's public key and signature in hex)\nconst SIGS: List[(Bytes, Bytes, Bool, Bytes, Str, Str)] = [\n")
	for i, w := range rows {
		k := priv(w.key)
		sig := hpkeMust(k.Sign(nil, w.msg, opts(w.ph, w.ctx)))
		switch i {
		case 0:
			if hex.EncodeToString(sig) != "98a70222f0b8121aa9d30f813d683f809e462b469c7ff87639499bb94e6dae4131f85042463c2a355a2003d062adf5aaa10b8c61e636062aaad11c2a26083406" {
				panic("ed25519: RFC 8032 7.3 disagrees")
			}
		case 2:
			if hex.EncodeToString(sig) != "55a4cc2f70a54e04288c5f4cd1e45a7bb520b36292911876cada7323198dd87a8b36950b95130022907a7fb7c4e9b2d5f6cca685a587b4b21f4b888e4e7edb0d" {
				panic("ed25519: RFC 8032 7.2 disagrees")
			}
		}
		pub := k.Public().(ed25519.PublicKey)
		if ed25519.VerifyWithOptions(pub, w.msg, sig, opts(w.ph, w.ctx)) != nil {
			panic("ed25519: Go refuses its own signature")
		}
		fmt.Fprintf(&o, "  (%s, %s, %v, %s, %s, %s),\n", xbytes(w.key), xbytes(w.msg), w.ph, xbytes(w.ctx), xq(hex.EncodeToString(pub)), xq(hex.EncodeToString(sig)))
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (message length, Ed25519ph, context length, Go's error)\nconst ERRORS: List[(Int, Bool, Int, Str)] = [\n")
	seed := make([]byte, 32)
	for _, c := range []struct {
		ml int
		ph bool
		cl int
	}{{0, true, 0}, {63, true, 0}, {65, true, 3}, {32, true, 0}, {64, true, 256}, {64, true, 300}, {10, false, 256}, {0, false, 1000}} {
		_, err := ed25519.NewKeyFromSeed(seed).Sign(nil, make([]byte, c.ml), opts(c.ph, make([]byte, c.cl)))
		verr := ed25519.VerifyWithOptions(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey), make([]byte, c.ml), make([]byte, 64), opts(c.ph, make([]byte, c.cl)))
		if err == nil || verr == nil || err.Error() != verr.Error() {
			panic("ed25519: Go's sign and verify errors differ")
		}
		fmt.Fprintf(&o, "  (%d, %v, %d, %s),\n", c.ml, c.ph, c.cl, xq(err.Error()))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn zeros(n: Int) -> Bytes {
  var b = BytesBuilder.with_capacity(n)
  for _ in 0..n {
    b.write_byte(0)
  }
  b.freeze()
}

fn flip(b: Bytes, i: Int) -> Bytes {
  var w = BytesBuilder.with_capacity(b.len())
  w.write(b)
  w.put_le(i, Int(b[i]) ^ 0xff, 1)
  w.freeze()
}

fn valid(pk: Bytes, msg: Bytes, sig: Bytes, ph: Bool, ctx: Bytes) -> Bool {
  crypto.ed25519_verify_with(pk, msg, sig, ph, ctx) ?? _ => false
}

test "Ed25519ph and Ed25519ctx are Go's (TestSignVerifyHashed, TestSignVerifyContext)" {
  var bad: List[Str] = []
  for (key, msg, ph, ctx, pubhex, want) in SIGS {
    let name = "key ${key.len()} msg ${msg.len()} ph ${ph} ctx ${ctx.len()}"
    let pk = Bytes.from_hex(pubhex) ?? b""
    let sig = crypto.ed25519_sign_with(key, msg, ph, ctx) ?? e => fault(e.msg)
    if sig.to_hex() != want {
      bad.push("${name}: sign")
      continue
    }
    if !valid(pk, msg, sig, ph, ctx) {
      bad.push("${name}: refused")
    }
    if ph == false && ctx.len() == 0 && sig != crypto.ed25519_sign(key, msg) {
      bad.push("${name}: not ed25519_sign")
    }
    // another variant, context, message or a changed signature: refused
    if valid(pk, msg, sig, !ph, ctx) && (msg.len() == 64 || ctx.len() > 0) {
      bad.push("${name}: the other variant")
    }
    if valid(pk, msg, sig, ph, ctx + b"x") || (ctx.len() > 0 && valid(pk, msg, sig, ph, ctx[1..ctx.len()])) {
      bad.push("${name}: another context")
    }
    if msg.len() > 0 && valid(pk, flip(msg, 0), sig, ph, ctx) {
      bad.push("${name}: another message")
    }
    if valid(pk, msg, flip(sig, 0), ph, ctx) || valid(pk, msg, flip(sig, 63), ph, ctx) || valid(pk, msg, sig[0..63], ph, ctx) {
      bad.push("${name}: a changed signature")
    }
    if ctx.len() > 0 && !ph && crypto.ed25519_verify(pk, msg, sig) {
      bad.push("${name}: pure Ed25519 took an Ed25519ctx signature")
    }
  }
  expect(bad) == []
}

test "the option errors are Go's" {
  var bad: List[Str] = []
  for (ml, ph, cl, want) in ERRORS {
    match crypto.ed25519_sign_with(zeros(32), zeros(ml), ph, zeros(cl)) {
      Ok(_) => bad.push("${ml} ${ph} ${cl}: signed")
      Err(e) => if e.msg != want {
        bad.push("${ml} ${ph} ${cl}: sign: ${e.msg}")
      }
    }
    let pk = crypto.ed25519_public_key(zeros(32))
    match crypto.ed25519_verify_with(pk, zeros(ml), zeros(64), ph, zeros(cl)) {
      Ok(_) => bad.push("${ml} ${ph} ${cl}: verified")
      Err(e) => if e.msg != want {
        bad.push("${ml} ${ph} ${cl}: verify: ${e.msg}")
      }
    }
  }
  expect(bad) == []
}

test "a non-canonical s is refused (TestMalleability)" {
  let msg = b"Test"
  let sig = Bytes.from_hex("7c38e026f29e14aabd059a0f2db8b0cd783040609a8be684db12f82a27774ab067654bce3832c2d76f8f6f5dafc08d9339d4eef676573336a5c51eb6f946b31d") ?? b""
  let pk = Bytes.from_hex("7d4d0e7f6153a69b6242b522abbee685fda4420f8834b108c3bdae369ef549fa") ?? b""
  expect(valid(pk, msg, sig, false, b"")) == false
  expect(valid(pk, msg, sig, false, b"ctx")) == false
  expect(valid(b"short", msg, sig, false, b"ctx")) == false
}
`)
	return o.String()
}
