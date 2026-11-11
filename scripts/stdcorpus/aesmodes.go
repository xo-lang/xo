package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
)

// AES block, CBC and CTR (decision 0605): crypto/aes and crypto/cipher on
// seeded keys (16, 24, 32 bytes), IVs and inputs; CTR's XORKeyStream over
// calls that split blocks, with counters near 2^64 and 2^128.

func aesModesCorpus() string {
	r := rand.New(rand.NewSource(605))
	rb := func(n int) []byte { b := make([]byte, n); r.Read(b); return b }
	key := func() []byte { return rb([]int{16, 24, 32}[r.Intn(3)]) }
	h := hex.EncodeToString
	var o strings.Builder
	o.WriteString(header("crypto/aes's Block and crypto/cipher's CBC and CTR on seeded keys, IVs and inputs, CTR split across calls and wrapping its counter"))
	o.WriteString("use std/crypto\n\n/// (key hex, Go's aes.NewCipher: ok or err TEXT)\nconst KEYS: List[(Str, Str)] = [\n")
	for _, n := range []int{0, 1, 8, 15, 16, 17, 23, 24, 25, 31, 32, 33, 48, 64} {
		got := "ok"
		if _, err := aes.NewCipher(rb(n)); err != nil {
			got = "err " + err.Error()
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(h(make([]byte, n))), xq(got))
	}
	o.WriteString("]\n\n/// (key hex, block hex, Go's Encrypt, Go's Decrypt)\nconst BLOCKS: List[(Str, Str, Str, Str)] = [\n")
	for k := 0; k < 150; k++ {
		kb, src := key(), rb(16)
		b, _ := aes.NewCipher(kb)
		e, d := make([]byte, 16), make([]byte, 16)
		b.Encrypt(e, src)
		b.Decrypt(d, src)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xq(h(kb)), xq(h(src)), xq(h(e)), xq(h(d)))
	}
	o.WriteString("]\n\n/// (key hex, iv hex, data hex, Go's CBC encryption, Go's CBC decryption of data)\nconst CBC: List[(Str, Str, Str, Str, Str)] = [\n")
	for k := 0; k < 150; k++ {
		kb, iv := key(), rb(16)
		n := 16 * []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 17}[r.Intn(10)]
		data := rb(n)
		b, _ := aes.NewCipher(kb)
		e, d := make([]byte, n), make([]byte, n)
		cipher.NewCBCEncrypter(b, iv).CryptBlocks(e, data)
		cipher.NewCBCDecrypter(b, iv).CryptBlocks(d, data)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(h(kb)), xq(h(iv)), xq(h(data)), xq(h(e)), xq(h(d)))
	}
	o.WriteString("]\n\n/// (key hex, iv hex, call sizes, data hex, Go's XORKeyStream over those calls)\nconst CTR: List[(Str, Str, List[Int], Str, Str)] = [\n")
	for k := 0; k < 200; k++ {
		kb, iv := key(), rb(16)
		switch r.Intn(4) {
		case 0:
			// the low half near 2^64: the carry into the high half
			for i := 8; i < 16; i++ {
				iv[i] = 0xff
			}
			iv[15] -= byte(r.Intn(4))
		case 1:
			// near 2^128: the counter wraps to zero
			for i := 0; i < 16; i++ {
				iv[i] = 0xff
			}
			iv[15] -= byte(r.Intn(4))
		}
		var sizes []int
		total := 0
		for c := 1 + r.Intn(5); c > 0; c-- {
			s := []int{0, 1, 5, 15, 16, 17, 31, 32, 33, 64, 65, 100}[r.Intn(12)]
			sizes = append(sizes, s)
			total += s
		}
		data := rb(total)
		b, _ := aes.NewCipher(kb)
		st := cipher.NewCTR(b, iv)
		out := make([]byte, total)
		at := 0
		var ss []string
		for _, s := range sizes {
			st.XORKeyStream(out[at:at+s], data[at:at+s])
			at += s
			ss = append(ss, fmt.Sprint(s))
		}
		fmt.Fprintf(&o, "  (%s, %s, [%s], %s, %s),\n", xq(h(kb)), xq(h(iv)), strings.Join(ss, ", "), xq(h(data)), xq(h(out)))
	}
	// (after the rows above, so they stay as they were) long calls with
	// the counter's low 32 bits a few blocks below 2^32: the CPU kernel,
	// which steps those bits only, must hand the carry back (2026-11-11)
	for k := 0; k < 60; k++ {
		kb, iv := key(), rb(16)
		for i := 12; i < 16; i++ {
			iv[i] = 0xff
		}
		iv[15] -= byte(r.Intn(24))
		var sizes []int
		total := 0
		for c := 1 + r.Intn(3); c > 0; c-- {
			s := []int{64, 65, 128, 200, 256, 300, 1000}[r.Intn(7)]
			sizes = append(sizes, s)
			total += s
		}
		data := rb(total)
		b, _ := aes.NewCipher(kb)
		st := cipher.NewCTR(b, iv)
		out := make([]byte, total)
		at := 0
		var ss []string
		for _, s := range sizes {
			st.XORKeyStream(out[at:at+s], data[at:at+s])
			at += s
			ss = append(ss, fmt.Sprint(s))
		}
		fmt.Fprintf(&o, "  (%s, %s, [%s], %s, %s),\n", xq(h(kb)), xq(h(iv)), strings.Join(ss, ", "), xq(h(data)), xq(h(out)))
	}
	o.WriteString("]\n")
	o.WriteString(aesModesTests)
	return o.String()
}

const aesModesTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn hx(s: Str) -> Bytes { Bytes.from_hex(s) ?? fault("hex") }

fn blk(key: Str) -> crypto.AesBlock { crypto.aes_block(hx(key)).ok() ?? fault("key") }

test "key sizes are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (key, want) in KEYS {
    let got = match crypto.aes_block(hx(key)) {
      Ok(_) => "ok"
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((key, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "blocks are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (key, src, enc, dec) in BLOCKS {
    let b = blk(key)
    let got = "${b.encrypt_block(hx(src)).to_hex()} ${b.decrypt_block(hx(src)).to_hex()} ${b.decrypt_block(b.encrypt_block(hx(src))) == hx(src)}"
    let want = "${enc} ${dec} true"
    if got != want {
      bad.push((src, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "CBC is Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (key, iv, data, enc, dec) in CBC {
    let b = blk(key)
    let e = crypto.cbc_encrypt(b, hx(iv), hx(data))
    let got = "${e.to_hex()} ${crypto.cbc_decrypt(b, hx(iv), hx(data)).to_hex()} ${crypto.cbc_decrypt(b, hx(iv), e) == hx(data)}"
    let want = "${enc} ${dec} true"
    if got != want {
      bad.push((data, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "CTR across calls is Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (key, iv, sizes, data, want) in CTR {
    var c = crypto.ctr(blk(key), hx(iv))
    let d = hx(data)
    var out = BytesBuilder.new()
    var at = 0
    for s in sizes {
      out.write(c.xor(d[at..at + s]))
      at += s
    }
    let got = out.freeze().to_hex()
    if got != want {
      bad.push((iv, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "a copied Ctr continues on its own" {
  let (key, iv, _s, data, want) = CTR.find(fn(t) { t.3.len() >= 64 }) ?? fault("row")
  let d = hx(data)
  var a = crypto.ctr(blk(key), hx(iv))
  let first = a.xor(d[0..7])
  var b = a
  let rest_a = a.xor(d[7..d.len()])
  let rest_b = b.xor(d[7..d.len()])
  expect(rest_a == rest_b) == true
  expect((first + rest_a).to_hex()) == want
}
`
