package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
)

// std/crypto's aes_gcm_key_with_nonce_size and aes_gcm_key_with_tag_size
// against Go's cipher.NewGCMWithNonceSize and NewGCMWithTagSize:
// gcm_test.go's aesGCMTests (the odd nonce and tag sizes among them),
// TestGCMInvalidTagSize and TestGCMCounterWrap (Copyright 2013 The Go
// Authors, BSD-style license), and seeded keys, nonces and messages over
// the sizes, sealed, opened, changed and cut.

func gcmSizesCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/cipher GCM and its gcm_test.go (Copyright 2013 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n")

	type row struct {
		key, nonce, pt, ad []byte
		tag                int
	}
	var rows []row
	for _, c := range goTestTable("crypto/cipher", "gcm_test.go", "aesGCMTests") {
		var f [5]string
		for i := range f {
			s, ok := strconvString(c.Elts[i])
			if !ok {
				panic("gcm: a test the generator cannot read")
			}
			f[i] = s
		}
		key, nonce, pt, ad := hpkeHex(f[0]), hpkeHex(f[1]), hpkeHex(f[2]), hpkeHex(f[3])
		tag := (len(f[4]) - len(f[2])) / 2
		if len(nonce) == 0 {
			continue
		}
		rows = append(rows, row{key, nonce, pt, ad, tag})
	}
	r := mrand.New(mrand.NewSource(605))
	rnd := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}
	for _, kl := range []int{16, 24, 32} {
		for _, ns := range []int{1, 8, 11, 12, 13, 16, 17, 32, 64} {
			for _, pl := range []int{0, 1, 15, 16, 17, 64, 100} {
				rows = append(rows, row{rnd(kl), rnd(ns), rnd(pl), rnd(pl % 20), 16})
			}
		}
		for _, tag := range []int{12, 13, 14, 15} {
			for _, pl := range []int{0, 1, 16, 33, 64, 100} {
				rows = append(rows, row{rnd(kl), rnd(12), rnd(pl), rnd(pl % 7), tag})
			}
		}
	}
	gcm := func(key []byte, ns, tag int) cipher.AEAD {
		b := hpkeMust(aes.NewCipher(key))
		if tag != 16 {
			return hpkeMust(cipher.NewGCMWithTagSize(b, tag))
		}
		return hpkeMust(cipher.NewGCMWithNonceSize(b, ns))
	}
	o.WriteString("\n/// (key, nonce, plaintext, additional data, tag size, Go's sealed message in hex)\nconst SEALS: List[(Bytes, Bytes, Bytes, Bytes, Int, Str)] = [\n")
	for _, w := range rows {
		ct := gcm(w.key, len(w.nonce), w.tag).Seal(nil, w.nonce, w.pt, w.ad)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %d, %s),\n", xbytes(w.key), xbytes(w.nonce), xbytes(w.pt), xbytes(w.ad), w.tag, xq(hex.EncodeToString(ct)))
	}
	o.WriteString("]\n")

	// Go's counter wrap cases: their tags, of a 273 byte zero message
	// under the zero key
	o.WriteString("\n/// (nonce, Go's tag in hex) of TestGCMCounterWrap\nconst WRAPS: List[(Bytes, Str)] = [\n")
	for _, w := range []struct{ nonce, tag string }{
		{"0fa72e25", "37e1948cdfff09fbde0c40ad99fee4a7"},
		{"afe05cc1", "438f3aa9fee5e54903b1927bca26bbdf"},
		{"9ffecbef", "7b88ca424df9703e9e8611071ec7e16e"},
		{"ffc3e5b3", "38d49c86e0abe853ac250e66da54c01a"},
		{"cfdd729d", "e08402eaac36a1a402e09b1bd56500e8"},
		{"010ae3d486", "5405bb490b1f95d01e2ba735687154bc"},
		{"01b1107a9d", "939a585f342e01e17844627492d44dbf"},
	} {
		got := gcm(make([]byte, 16), len(w.nonce)/2, 16).Seal(nil, hpkeHex(w.nonce), make([]byte, 16*17+1), nil)
		if hex.EncodeToString(got[16*17+1:]) != w.tag {
			panic("gcm: Go's counter wrap case disagrees")
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(hpkeHex(w.nonce)), xq(w.tag))
	}
	o.WriteString("]\n")

	// errors
	o.WriteString("\n/// (key length, nonce size, tag size, Go's error)\nconst ERRORS: List[(Int, Int, Int, Str)] = [\n")
	for _, c := range [][3]int{{16, 0, 16}, {16, -1, 16}, {15, 12, 16}, {33, 8, 16}, {16, 12, 0}, {16, 12, 1}, {16, 12, 11}, {16, 12, 17}, {20, 12, 12}} {
		var err error
		b, err := aes.NewCipher(make([]byte, c[0]))
		if err == nil {
			if c[2] != 16 {
				_, err = cipher.NewGCMWithTagSize(b, c[2])
			} else {
				_, err = cipher.NewGCMWithNonceSize(b, c[1])
			}
		}
		if err == nil {
			panic("gcm: no error")
		}
		fmt.Fprintf(&o, "  (%d, %d, %d, %s),\n", c[0], c[1], c[2], xq(err.Error()))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn key_of(key: Bytes, ns: Int, tag: Int) -> crypto.AeadKey ! crypto.AeadErr {
  if tag != 16 { crypto.aes_gcm_key_with_tag_size(key, tag) } else { crypto.aes_gcm_key_with_nonce_size(key, ns) }
}

fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "seal and open are Go's for every nonce and tag size (Go's TestAESGCM)" {
  var bad: List[Str] = []
  for (key, nonce, pt, ad, tag, want) in SEALS {
    let name = "key ${key.len()} nonce ${nonce.len()} tag ${tag} pt ${pt.len()}"
    let k = key_of(key, nonce.len(), tag) ?? e => fault(e.msg)
    if k.nonce_size() != nonce.len() || k.overhead() != tag || k.alg() != crypto.Aead.AesGcm {
      bad.push("${name}: sizes")
    }
    let ct = k.seal(nonce, pt, ad)
    if ct.to_hex() != want {
      bad.push("${name}: seal")
      continue
    }
    if (k.open(nonce, ct, ad) ?? _ => b"!") != pt {
      bad.push("${name}: open")
    }
    // a changed byte, the tag cut, the wrong data: refused
    var b = BytesBuilder.with_capacity(ct.len())
    b.write(ct)
    b.put_le(ct.len() - 1, Int(ct[ct.len() - 1]) ^ 1, 1)
    if k.open(nonce, b.freeze(), ad).is_ok() || k.open(nonce, ct[0..ct.len() - 1], ad).is_ok() || k.open(nonce, ct, ad + b"x").is_ok() {
      bad.push("${name}: opened a changed message")
    }
    if k.open(nonce, ct[0..tag - 1], ad).is_ok() {
      bad.push("${name}: opened a message shorter than the tag")
    }
    let sr = k.open_random(nonce + ct, ad) ?? _ => b"!"
    if sr != pt {
      bad.push("${name}: open_random")
    }
  }
  expect(show(bad)) == []
}

test "the counter's last 32 bits wrap (Go's TestGCMCounterWrap)" {
  var z = BytesBuilder.with_capacity(273)
  for _ in 0..273 {
    z.write_byte(0)
  }
  let pt = z.freeze()
  for (nonce, want) in WRAPS {
    let k = crypto.aes_gcm_key_with_nonce_size(pt[0..16], nonce.len()) ?? e => fault(e.msg)
    let got = k.seal(nonce, pt, b"")
    expect(got[273..got.len()].to_hex()) == want
    expect((k.open(nonce, got, b"") ?? _ => b"!")) == pt
  }
}

test "the size errors are Go's (Go's TestGCMInvalidTagSize)" {
  var bad: List[Str] = []
  for (kl, ns, tag, want) in ERRORS {
    var b = BytesBuilder.with_capacity(kl)
    for _ in 0..kl {
      b.write_byte(0)
    }
    match key_of(b.freeze(), ns, tag) {
      Ok(_) => bad.push("${kl} ${ns} ${tag}: no error")
      Err(e) => if e.msg != want {
        bad.push("${kl} ${ns} ${tag}: ${e.msg}")
      }
    }
  }
  expect(bad) == []
}

test "aead_key's keys keep their sizes" {
  let k = crypto.aead_key(crypto.Aead.AesGcm, b"0123456789abcdef") ?? e => fault(e.msg)
  expect(k.nonce_size()) == 12
  expect(k.overhead()) == 16
  let c = crypto.aead_key(crypto.Aead.ChaCha20Poly1305, b"0123456789abcdef0123456789abcdef") ?? e => fault(e.msg)
  expect(c.nonce_size()) == 12
  expect(c.overhead()) == 16
  let j = crypto.aead_key(crypto.Aead.Aes128CbcHmacSha256, b"0123456789abcdef0123456789abcdef") ?? e => fault(e.msg)
  expect(j.nonce_size()) == 16
  expect(j.overhead()) == 32
}
`)
	return o.String()
}
