package main

import (
	"crypto/sha3"
	"encoding/hex"
	"fmt"
	"strings"
)

// std/crypto's Shake against Go's crypto/sha3: SHAKE128, SHAKE256,
// cSHAKE128 and cSHAKE256 over function names and customizations that
// cross the rate, inputs at the rates' boundaries, written in two pieces
// and read in two; then sha3_test.go's TestSqueezing, TestReset,
// TestCSHAKEAccumulated and TestCSHAKELargeS (Copyright 2014 The Go
// Authors, BSD-style license) with their expected outputs.

// shakeNew is Go's constructor of kind k (0 shake128, 1 shake256, 2
// cshake128, 3 cshake256).
func shakeNew(k int, n, s []byte) *sha3.SHAKE {
	switch k {
	case 0:
		return sha3.NewSHAKE128()
	case 1:
		return sha3.NewSHAKE256()
	case 2:
		return sha3.NewCSHAKE128(n, s)
	}
	return sha3.NewCSHAKE256(n, s)
}

func shakeCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/sha3 SHAKE and cSHAKE, and the tests of its sha3_test.go (Copyright 2014 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n\n" + hsBufFn)
	o.WriteString(`
fn make(kind: Int, n: Bytes, s: Bytes) -> crypto.Shake {
  match kind {
    0 => crypto.shake128()
    1 => crypto.shake256()
    2 => crypto.cshake128(n, s)
    _ => crypto.cshake256(n, s)
  }
}
`)
	type ns struct{ n, s []byte }
	pairs := []ns{
		{nil, nil},
		{nil, []byte("Email Signature")},
		{[]byte("CSHAKE128"), []byte("CustomString")},
		{[]byte{0x99}, nil},
		{nil, []byte{0x99, 0x98}},
		{hsBuf(160, 3), hsBuf(9, 4)},
		{hsBuf(200, 5), hsBuf(300, 6)},
		{nil, hsBuf(1000, 8)},
	}
	inLens := []int{0, 1, 7, 8, 9, 103, 104, 135, 136, 137, 167, 168, 169, 200, 336, 1000}
	outLens := []int{0, 1, 8, 32, 64, 135, 136, 137, 168, 169, 400}
	o.WriteString("\n/// The (function name, customization) pairs of SHAKES.\nconst PAIRS: List[(Bytes, Bytes)] = [\n")
	for _, p := range pairs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(p.n), xbytes(p.s))
	}
	o.WriteString("]\n")
	o.WriteString("\n/// (kind, index in PAIRS, input length, input seed, output length, Go's output in hex)\nconst SHAKES: List[(Int, Int, Int, Int, Int, Str)] = [\n")
	row := 0
	for k := 0; k < 4; k++ {
		ps := pairs
		if k < 2 {
			ps = pairs[:1]
		}
		for pi, p := range ps {
			for i, n := range inLens {
				for j := 0; j < 2; j++ {
					m := outLens[(i*3+j*5+row)%len(outLens)]
					row++
					seed := 11 + row
					x := shakeNew(k, p.n, p.s)
					x.Write(hsBuf(n, seed))
					out := make([]byte, m)
					x.Read(out)
					fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d, %s),\n", k, pi, n, seed, m, xq(hex.EncodeToString(out)))
				}
			}
		}
	}
	o.WriteString("]\n")

	// the one shot functions
	o.WriteString("\n/// (input length, seed, output length, Go's SumSHAKE128 and SumSHAKE256 in hex)\nconst SUMS: List[(Int, Int, Int, Str, Str)] = [\n")
	for i, n := range inLens {
		m := outLens[(i*7)%len(outLens)]
		d := hsBuf(n, 90+i)
		fmt.Fprintf(&o, "  (%d, %d, %d, %s, %s),\n", n, 90+i, m, xq(hex.EncodeToString(sha3.SumSHAKE128(d, m))), xq(hex.EncodeToString(sha3.SumSHAKE256(d, m))))
	}
	o.WriteString("]\n")

	// Go's testString of sha3_test.go
	o.WriteString(`
const TEST_STRING = "brekeccakkeccak koax koax"

fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "Shake is Go's sha3.SHAKE, written in two pieces and read in two" {
  var bad: List[Str] = []
  for (kind, pi, len, seed, m, want) in SHAKES {
    let (n, s) = PAIRS[pi]
    let input = buf(len, seed)
    var x = make(kind, n, s)
    x.write(input[0..len / 2])
    x.write(input[len / 2..len])
    let a = x.read(m / 3)
    let got = (a.to_hex() + x.read(m - m / 3).to_hex())
    if got != want {
      bad.push("kind ${kind} n ${n.len()} s ${s.len()} input ${len} output ${m}")
    }
    // one write and one read
    var y = make(kind, n, s)
    y.write(input)
    if y.read(m).to_hex() != want {
      bad.push("whole: kind ${kind} n ${n.len()} s ${s.len()} input ${len} output ${m}")
    }
  }
  expect(show(bad)) == []
}

test "sum_shake128 and sum_shake256 are Go's SumSHAKE128 and SumSHAKE256" {
  var bad: List[Str] = []
  for (len, seed, m, want128, want256) in SUMS {
    let d = buf(len, seed)
    if crypto.sum_shake128(d, m).to_hex() != want128 {
      bad.push("128: input ${len} output ${m}")
    }
    if crypto.sum_shake256(d, m).to_hex() != want256 {
      bad.push("256: input ${len} output ${m}")
    }
  }
  expect(show(bad)) == []
}

test "squeezing a byte at a time is one read (Go's TestSqueezing)" {
  let cases = [(2, b"", b""), (3, b"", b""), (2, b"CSHAKE128", b"CustomString"), (3, b"CSHAKE256", b"CustomString")]
  for (kind, n, s) in cases {
    var d0 = make(kind, n, s)
    d0.write(TEST_STRING.bytes())
    let want = d0.read(32)
    var d1 = make(kind, n, s)
    d1.write(TEST_STRING.bytes())
    var got = BytesBuilder.with_capacity(32)
    for _ in 0..32 {
      expect(d1.read(0).len()) == 0
      got.write(d1.read(1))
    }
    expect(got.freeze().to_hex()) == want.to_hex()
  }
}

test "reset starts over, cSHAKE keeping its customization (Go's TestReset)" {
  var b = BytesBuilder.with_capacity(256)
  for i in 0..256 {
    b.write_byte(UInt8(i))
  }
  let data = b.freeze()
  for kind in [2, 3] {
    var c = make(kind, b"", b"\x99\x98")
    c.write(data)
    let out1 = c.read(32)
    c.reset()
    c.write(data)
    expect(c.read(32).to_hex()) == out1.to_hex()
  }
}

test "a copy of a Shake is independent" {
  var a = crypto.cshake128(b"N", b"S")
  a.write(b"abc")
  var b = a
  b.write(b"def")
  let ra = a.read(16)
  var c = crypto.cshake128(b"N", b"S")
  c.write(b"abc")
  expect(ra.to_hex()) == c.read(16).to_hex()
  var d = crypto.cshake128(b"N", b"S")
  d.write(b"abcdef")
  expect(b.read(16).to_hex()) == d.read(16).to_hex()
}

test "block_size is the rate" {
  expect(crypto.shake128().block_size()) == 168
  expect(crypto.shake256().block_size()) == 136
  expect(crypto.cshake128(b"a", b"").block_size()) == 168
  expect(crypto.cshake256(b"", b"b").block_size()) == 136
}

fn accumulated(kind: Int, rate: Int) -> Str {
  var rnd = make(kind, b"", b"")
  var acc = make(kind, b"", b"")
  for n in 0..200 {
    let nn = rnd.read(n)
    for s in 0..200 {
      let ss = rnd.read(s)
      var c = make(kind, nn, ss)
      c.write(rnd.read(100))
      acc.write(c.read(200))
      c.reset()
      c.write(rnd.read(rate))
      acc.write(c.read(200))
      c.reset()
      c.write(rnd.read(200))
      acc.write(c.read(200))
    }
  }
  acc.read(32).to_hex()
}

test "cSHAKE over 40000 function names and customizations (Go's TestCSHAKEAccumulated)" {
  expect(accumulated(2, 168)) == "bb14f8657c6ec5403d0b0e2ef3d3393497e9d3b1a9a9e8e6c81dbaa5fd809252"
  expect(accumulated(3, 136)) == "0baaf9250c6e25f0c14ea5c7f9bfde54c8a922c8276437db28f3895bdf6eeeef"
}

test "cSHAKE with a customization over 2^32 bits (Go's TestCSHAKELargeS, go.dev/issue/66232)" {
  var rnd = crypto.shake128()
  let s = rnd.read(536870912 + 1000)
  var c = crypto.cshake128(b"", s)
  c.write(rnd.read(1000))
  expect(c.read(32).to_hex()) == "2cb9f237767e98f2614b8779cf096a52da9b3a849280bbddec820771ae529cf0"
}
`)
	return o.String()
}
