package main

import (
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand"
	"runtime"
	"strings"
)

// std/big's random_below and random_prime and std/crypto's random_text
// against Go's crypto/rand.Int, Prime and Text: util_test.go's TestInt,
// TestIntReads, TestIntMask, TestPrimeSmall and TestPrimeBitsLt2 and
// text_test.go's TestText (Copyright 2009 and 2024 The Go Authors,
// BSD-style license). Int and Prime read a seeded stream through a reader
// that records how much they took; Go's Prime first skips a byte half the
// time (randutil.MaybeReadByte, with cryptocustomrand=1), which the reader
// sees on the stack and drops from the stream Xo is given; each row is
// retried until the skip alternates, so the rows do not change.

// randRecorder hands out a stream and counts what Go's Int and Prime read
// from it, apart from MaybeReadByte's one byte.
type randRecorder struct {
	b        []byte
	at, skip int
}

func (r *randRecorder) Read(p []byte) (int, error) {
	if r.at+len(p) > len(r.b) {
		panic("randbig: the stream ran out")
	}
	n := copy(p, r.b[r.at:])
	pc := make([]uintptr, 8)
	k := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:k])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.Function, "randutil.MaybeReadByte") {
			if r.at != 0 || n != 1 {
				panic("randbig: MaybeReadByte past the start")
			}
			r.skip = 1
		}
		if !more {
			break
		}
	}
	r.at += n
	return n, nil
}

func randBigCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/rand and its util_test.go and text_test.go (Copyright 2009 and 2024 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/big\nuse std/crypto\n")
	r := mrand.New(mrand.NewSource(2009))
	rnd := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}

	// Int: TestInt's 128 to 139, TestIntReads' powers of two, TestIntMask's
	// 1 to 256, and numbers about the byte and limb edges
	var maxes []*big.Int
	for n := int64(1); n <= 256; n++ {
		maxes = append(maxes, big.NewInt(n))
	}
	for i := 9; i < 32; i++ {
		maxes = append(maxes, big.NewInt(1<<i))
	}
	for _, s := range []string{"65535", "65536", "65537", "9223372036854775807", "9223372036854775808", "18446744073709551615", "18446744073709551616", "18446744073709551617", "170141183460469231731687303715884105727", "340282366920938463463374607431768211456", "1000000000000000000000000000000"} {
		m, _ := new(big.Int).SetString(s, 10)
		maxes = append(maxes, m)
	}
	for _, bits := range []int{70, 129, 255, 300, 521, 1024} {
		maxes = append(maxes, new(big.Int).SetBytes(rnd((bits+7)/8)))
	}
	o.WriteString("\n/// (max in decimal, the stream, Go's Int in decimal, bytes Go read)\nconst INTS: List[(Str, Bytes, Str, Int)] = [\n")
	for _, m := range maxes {
		for rep := 0; rep < 3; rep++ {
			k := (new(big.Int).Sub(m, big.NewInt(1)).BitLen() + 7) / 8
			rec := &randRecorder{b: rnd(k * 12)}
			n := hpkeMust(cryptorand.Int(rec, m))
			fmt.Fprintf(&o, "  (%s, %s, %s, %d),\n", xq(m.String()), xbytes(rec.b[:rec.at]), xq(n.String()), rec.at)
			if m.BitLen() > 64 {
				break
			}
		}
	}
	o.WriteString("]\n")

	// TestIntMask's one byte readers: each value below max, read back
	o.WriteString("\n/// TestIntMask: every byte below max, from a reader of that byte alone, is the result\n")

	// Prime: TestPrimeSmall's 2 to 10 bits, and the byte and limb edges
	o.WriteString("\n/// (bits, the stream after Go's skipped byte, Go's prime in decimal, bytes Go read after it)\nconst PRIMES: List[(Int, Bytes, Str, Int)] = [\n")
	skips := [2]int{}
	row := 0
	for _, bits := range []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 15, 16, 17, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 512, 1024} {
		reps := 3
		if bits > 128 {
			reps = 1
		}
		for rep := 0; rep < reps; rep++ {
			// MaybeReadByte flips a coin: try again until it skipped on
			// odd rows and not on even ones, so the rows are the same on
			// every run
			stream := rnd(((bits + 7) / 8) * (bits*2 + 40))
			var rec *randRecorder
			var p *big.Int
			for tries := 0; rec == nil || rec.skip != row%2; tries++ {
				if tries == 200 {
					panic("randbig: MaybeReadByte never gave the wanted skip")
				}
				rec = &randRecorder{b: stream}
				p = hpkeMust(cryptorand.Prime(rec, bits))
			}
			if p.BitLen() != bits {
				panic("randbig: Go's prime is not that long")
			}
			skips[rec.skip]++
			row++
			fmt.Fprintf(&o, "  (%d, %s, %s, %d),\n", bits, xbytes(rec.b[rec.skip:rec.at]), xq(p.String()), rec.at-rec.skip)
		}
	}
	o.WriteString("]\n")
	if skips[0] == 0 || skips[1] == 0 {
		panic(fmt.Sprintf("randbig: MaybeReadByte skipped %d of %d times; is cryptocustomrand=1 set?", skips[1], skips[0]+skips[1]))
	}
	_, err := cryptorand.Prime(&randRecorder{b: rnd(8)}, 1)
	fmt.Fprintf(&o, "\n/// Go's error for a prime under 2 bits (TestPrimeBitsLt2)\nconst PRIME_ERR: Str = %s\n", xq(err.Error()))

	// Text takes no reader, so the rows are its loop over seeded bytes
	o.WriteString("\n/// (26 bytes, Go's Text over them: each byte mod 32 in the base32 alphabet)\nconst TEXTS: List[(Bytes, Str)] = [\n")
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	for i := 0; i < 40; i++ {
		src := rnd(26)
		if i < 8 {
			for j := range src {
				src[j] = byte(i*32 + j)
			}
		}
		t := make([]byte, 26)
		for j := range src {
			t[j] = alphabet[src[j]%32]
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(src), xq(string(t)))
	}
	o.WriteString("]\n")
	if len(cryptorand.Text()) != 26 {
		panic("randbig: Go's Text is not 26 characters")
	}

	o.WriteString(`
/// A Rand that gives fixed bytes and counts them.
struct Fixed {
  b: Bytes
  at: Mutex[Int]
}

fn fixed(b: Bytes) -> Fixed { Fixed{b: b, at: Mutex[Int].new(0)} }

fn (s: Fixed) int(lo: Int, hi: Int) -> Int uses rand { lo }
fn (s: Fixed) float() -> Float uses rand { 0.5 }
fn (s: Fixed) bool() -> Bool uses rand { false }
fn (s: Fixed) shuffle[T](xs: List[T]) -> List[T] uses rand { xs }
fn (s: Fixed) bytes(n: Int) -> Bytes uses rand {
  let i = s.at.with(fn(var x: Int) -> Int {
    let at = x
    x = x + n
    at
  })
  s.b[i..i + n]
}
fn (s: Fixed) used() -> Int { s.at.with(fn(var x: Int) -> Int { x }) }

fn one(i: Int) -> Bytes {
  var b = BytesBuilder.with_capacity(1)
  b.write_byte(UInt8(i))
  b.freeze()
}

fn num(s: Str) -> big.BigInt { big.parse(s) ?? e => fault(e.msg) }

fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "random_below is Go's rand.Int, reading what Go reads (TestInt, TestIntReads)" {
  var bad: List[Str] = []
  for (max, stream, want, read) in INTS {
    let r = fixed(stream)
    let got = big.random_below(r, num(max))
    if got.text() != want || r.used() != read {
      bad.push("max ${max}: ${got} after ${r.used()} bytes, Go ${want} after ${read}")
    }
  }
  expect(show(bad)) == []
}

test "random_below of a one byte stream below max is that byte (TestIntMask)" {
  var bad: List[Str] = []
  for max in 1..257 {
    for i in 0..max {
      let r = fixed(one(i))
      let got = big.random_below(r, big.from_int(max))
      if got != big.from_int(i) {
        bad.push("max ${max}: ${got}, want ${i}")
      }
    }
  }
  expect(show(bad)) == []
}

test "random_below of 1 reads nothing and is 0" {
  let r = fixed(b"")
  expect(big.random_below(r, big.from_int(1))) == big.from_int(0)
  expect(r.used()) == 0
}

test "random_prime is Go's rand.Prime, reading what Go reads (TestPrimeSmall)" {
  var bad: List[Str] = []
  for (bits, stream, want, read) in PRIMES {
    let r = fixed(stream)
    match big.random_prime(r, bits) {
      Ok(p) => {
        if p.text() != want || r.used() != read || p.bit_len() != bits || !p.probably_prime(20) {
          bad.push("bits ${bits}: ${p} after ${r.used()} bytes, Go ${want} after ${read}")
        }
      }
      Err(e) => bad.push("bits ${bits}: ${e.msg}")
    }
  }
  expect(show(bad)) == []
}

test "random_prime under 2 bits is Go's error (TestPrimeBitsLt2)" {
  for bits in [-1, 0, 1] {
    match big.random_prime(fixed(b"\x00\x00"), bits) {
      Ok(p) => expect("${bits}: ${p}") == "an error"
      Err(e) => expect(e.msg) == PRIME_ERR
    }
  }
}

test "random_text is Go's rand.Text over the Rand's bytes" {
  var bad: List[Str] = []
  for (src, want) in TEXTS {
    let r = fixed(src)
    let got = crypto.random_text(r)
    if got != want || r.used() != 26 {
      bad.push("${src.to_hex()}: ${got}")
    }
  }
  expect(show(bad)) == []
}

test "random_text gives every base32 character at every index (TestText)" {
  let r = os.rand
  let alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".bytes()
  var seen: Set[Str] = #{}
  // index * 256 + character, for each seen
  var hits: Set[Int] = #{}
  var rounds = 0
  var done = false
  while !done && rounds < 1000 {
    let s = crypto.random_text(r)
    expect(s.len()) == 26
    expect(seen.contains(s)) == false
    seen.add(s)
    let b = s.bytes()
    for i in 0..b.len() {
      expect(alphabet.find(b[i..i + 1]).is_some()) == true
      hits.add(i * 256 + Int(b[i]))
    }
    done = hits.len() == 26 * 32
    rounds = rounds + 1
  }
  expect(done) == true
}
`)
	return o.String()
}
