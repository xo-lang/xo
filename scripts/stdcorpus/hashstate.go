package main

import (
	"crypto/hmac"
	"crypto/sha3"
	"encoding"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
)

// std/crypto's Hasher.block_size, size, reset, marshal_binary and
// unmarshal_binary, and Shake's marshal_binary and unmarshal_binary,
// against Go's hashes: sha256_test.go's and sha512_test.go's
// TestGoldenMarshal, TestMarshalMismatch and TestBlockSize, sha3_test.go's
// TestMarshalUnmarshal (Copyright 2009 and 2014 The Go Authors,
// BSD-style license), over every Hash and message lengths at the block
// boundaries, and Go's errors for every mismatch.

func hashStateCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto hashes' MarshalBinary and their tests (Copyright 2009 and 2014 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n\nconst HASHES: List[crypto.Hash] = [")
	for i, h := range cryptoHashes {
		if i > 0 {
			o.WriteString(", ")
		}
		o.WriteString("crypto.Hash." + h.enum)
	}
	o.WriteString("]\n")
	marshal := func(x any) []byte { return hpkeMust(x.(encoding.BinaryMarshaler).MarshalBinary()) }
	r := mrand.New(mrand.NewSource(2014))
	rnd := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}

	o.WriteString("\n/// (index in HASHES, Go's block size, Go's size)\nconst SIZES: List[(Int, Int, Int)] = [\n")
	for i, h := range cryptoHashes {
		fmt.Fprintf(&o, "  (%d, %d, %d),\n", i, h.new().BlockSize(), h.new().Size())
	}
	o.WriteString("]\n")

	// TestGoldenMarshal: half written, the state, then the rest and the sum
	o.WriteString("\n/// (index in HASHES, message, bytes written before the state, Go's state and digest in hex)\nconst STATES: List[(Int, Bytes, Int, Str, Str)] = [\n")
	for i, h := range cryptoHashes {
		bs := h.new().BlockSize()
		for _, n := range []int{0, 1, 3, bs - 1, bs, bs + 1, 2*bs + 3, 300} {
			for _, half := range []int{0, n / 2, n} {
				msg := rnd(n)
				x := h.new()
				x.Write(msg[:half])
				st := marshal(x)
				x.Write(msg[half:])
				fmt.Fprintf(&o, "  (%d, %s, %d, %s, %s),\n", i, xbytes(msg), half, xq(hex.EncodeToString(st)), xq(hex.EncodeToString(x.Sum(nil))))
			}
		}
	}
	o.WriteString("]\n")

	// TestMarshalMismatch, and cut, grown and changed states
	o.WriteString("\n/// (index in HASHES the state is for, its state, index unmarshaled into, Go's error or \"\")\nconst MISMATCH: List[(Int, Bytes, Int, Str)] = [\n")
	for i, h := range cryptoHashes {
		x := h.new()
		x.Write([]byte("abc"))
		st := marshal(x)
		cases := [][]byte{st, st[:len(st)-1], append(append([]byte(nil), st...), 0), st[:3], nil}
		bad := append([]byte(nil), st...)
		bad[0] ^= 1
		cases = append(cases, bad)
		if len(st) == 207 {
			for _, k := range []int{4, 205, 206} {
				b := append([]byte(nil), st...)
				b[k] = 0xff
				cases = append(cases, b)
			}
		}
		for j, g := range cryptoHashes {
			for c, s := range cases {
				if c > 0 && j != i {
					continue
				}
				msg := ""
				if err := g.new().(encoding.BinaryUnmarshaler).UnmarshalBinary(s); err != nil {
					msg = err.Error()
				}
				fmt.Fprintf(&o, "  (%d, %s, %d, %s),\n", i, xbytes(s), j, xq(msg))
			}
		}
	}
	o.WriteString("]\n")

	// SHAKE and cSHAKE (TestMarshalUnmarshal): written, read, the state,
	// then more read, and the output after a reset of the unmarshaled one
	type shk struct {
		name string
		xo   string
		new  func() *sha3.SHAKE
	}
	shakes := []shk{
		{"shake128", "crypto.shake128()", sha3.NewSHAKE128},
		{"shake256", "crypto.shake256()", sha3.NewSHAKE256},
		{"cshake128", `crypto.cshake128(b"N", b"S")`, func() *sha3.SHAKE { return sha3.NewCSHAKE128([]byte("N"), []byte("S")) }},
		{"cshake256", `crypto.cshake256(b"", b"custom")`, func() *sha3.SHAKE { return sha3.NewCSHAKE256(nil, []byte("custom")) }},
		// a cSHAKE128 of another customization: unmarshal takes the
		// state's init block, and reset goes back to it
		{"cshake128 other", `crypto.cshake128(b"other", b"")`, func() *sha3.SHAKE { return sha3.NewCSHAKE128([]byte("other"), nil) }},
	}
	o.WriteString("\nfn shake_of(i: Int) -> crypto.Shake {\n  match i {\n")
	for i, s := range shakes {
		fmt.Fprintf(&o, "    %d => %s\n", i, s.xo)
	}
	o.WriteString("    _ => fault(\"shake\")\n  }\n}\n")
	o.WriteString("\n/// (index in shake_of, message, bytes read before the state, Go's state, Go's next 200 bytes and 50 after a reset, in hex)\nconst SHAKES: List[(Int, Bytes, Int, Str, Str, Str)] = [\n")
	for i, s := range shakes[:4] {
		for _, n := range []int{0, 1, 135, 136, 168, 300} {
			for _, rd := range []int{0, 1, 168, 200} {
				msg := rnd(n)
				x := s.new()
				x.Write(msg)
				pre := make([]byte, rd)
				x.Read(pre)
				st := hpkeMust(x.MarshalBinary())
				y := s.new()
				if i == 2 {
					y = shakes[4].new()
				}
				if err := y.UnmarshalBinary(st); err != nil {
					panic(err)
				}
				next := make([]byte, 200)
				y.Read(next)
				y.Reset()
				y.Write([]byte("after"))
				after := make([]byte, 50)
				y.Read(after)
				fmt.Fprintf(&o, "  (%d, %s, %d, %s, %s, %s),\n", i, xbytes(msg), rd, xq(hex.EncodeToString(st)), xq(hex.EncodeToString(next)), xq(hex.EncodeToString(after)))
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString("\n/// (index the state is for, its state, index unmarshaled into, Go's error or \"\")\nconst SHAKE_MISMATCH: List[(Int, Bytes, Int, Str)] = [\n")
	for i, s := range shakes {
		x := s.new()
		x.Write([]byte("abc"))
		st := hpkeMust(x.MarshalBinary())
		for j, t := range shakes {
			for c, b := range [][]byte{st, st[:206], st[:207]} {
				if c > 0 && j != i {
					continue
				}
				msg := ""
				if err := t.new().UnmarshalBinary(b); err != nil {
					msg = err.Error()
				}
				fmt.Fprintf(&o, "  (%d, %s, %d, %s),\n", i, xbytes(b), j, xq(msg))
			}
		}
	}
	o.WriteString("]\n")

	// HMAC reset
	o.WriteString("\n/// (index in HASHES, key, message, Go's HMAC of the message after a reset, in hex)\nconst HMACS: List[(Int, Bytes, Bytes, Str)] = [\n")
	for i, h := range cryptoHashes {
		key, msg := rnd(i*13%70), rnd(i*7%40)
		m := hmac.New(h.new, key)
		m.Write([]byte("before the reset"))
		m.Reset()
		m.Write(msg)
		fmt.Fprintf(&o, "  (%d, %s, %s, %s),\n", i, xbytes(key), xbytes(msg), xq(hex.EncodeToString(m.Sum(nil))))
	}
	o.WriteString("]\n")

	o.WriteString(`
test "block_size and size are Go's (TestBlockSize)" {
  for (i, bs, size) in SIZES {
    let x = crypto.hasher(HASHES[i])
    expect(x.block_size()) == bs
    expect(x.size()) == size
  }
}

test "marshal_binary is Go's MarshalBinary and unmarshal_binary resumes Go's state (TestGoldenMarshal)" {
  var bad: List[Str] = []
  for (i, msg, half, st, want) in STATES {
    let name = "${HASHES[i]:?} ${msg.len()} at ${half}"
    var x = crypto.hasher(HASHES[i])
    x.write(msg[0..half])
    let got = x.marshal_binary() ?? e => fault(e.msg)
    if got.to_hex() != st {
      bad.push("${name}: marshal")
    }
    let state = Bytes.from_hex(st) ?? b""
    var y = crypto.hasher(HASHES[i]).unmarshal_binary(state) ?? e => fault("${name}: ${e.msg}")
    if (y.marshal_binary() ?? _ => b"") != state {
      bad.push("${name}: the state again")
    }
    // SHA-3's 207 byte state keeps no length
    let w = if state.len() == 207 { 0 } else { half }
    if y.written() != w {
      bad.push("${name}: written ${y.written()}")
    }
    y.write(msg[half..msg.len()])
    if y.sum().to_hex() != want {
      bad.push("${name}: sum")
    }
    y.reset()
    if y.sum() != crypto.digest(HASHES[i], b"") || y.written() != 0 {
      bad.push("${name}: reset")
    }
  }
  expect(bad) == []
}

test "unmarshal_binary's errors are Go's (TestMarshalMismatch)" {
  var bad: List[Str] = []
  for (i, st, j, want) in MISMATCH {
    let got = match crypto.hasher(HASHES[j]).unmarshal_binary(st) {
      Ok(_) => ""
      Err(e) => e.msg
    }
    if got != want {
      bad.push("${HASHES[i]:?} ${st.len()} into ${HASHES[j]:?}: ${got}, want ${want}")
    }
  }
  expect(bad) == []
}

test "Shake's state is Go's (TestMarshalUnmarshal)" {
  var bad: List[Str] = []
  for (i, msg, rd, st, next, after) in SHAKES {
    let name = "shake ${i} ${msg.len()} read ${rd}"
    var x = shake_of(i)
    x.write(msg)
    let _ = x.read(rd)
    if x.marshal_binary().to_hex() != st {
      bad.push("${name}: marshal")
    }
    var y = shake_of(if i == 2 { 4 } else { i }).unmarshal_binary(Bytes.from_hex(st) ?? b"") ?? e => fault("${name}: ${e.msg}")
    if y.read(200).to_hex() != next {
      bad.push("${name}: read")
    }
    y.reset()
    y.write(b"after")
    if y.read(50).to_hex() != after {
      bad.push("${name}: reset")
    }
  }
  for (i, st, j, want) in SHAKE_MISMATCH {
    let got = match shake_of(j).unmarshal_binary(st) {
      Ok(_) => ""
      Err(e) => e.msg
    }
    if got != want {
      bad.push("shake ${i} ${st.len()} into ${j}: ${got}, want ${want}")
    }
  }
  expect(bad) == []
}

test "reset keeps the HMAC key, and an HMAC has no binary state" {
  var bad: List[Str] = []
  for (i, key, msg, want) in HMACS {
    var x = crypto.hmac_hasher(HASHES[i], key)
    x.write(b"before the reset")
    x.reset()
    x.write(msg)
    if x.sum().to_hex() != want || x.written() != msg.len() {
      bad.push("${HASHES[i]:?}: reset")
    }
    if x.marshal_binary().is_ok() || x.unmarshal_binary(b"").is_ok() {
      bad.push("${HASHES[i]:?}: an HMAC state")
    }
  }
  expect(bad) == []
}
`)
	return o.String()
}
