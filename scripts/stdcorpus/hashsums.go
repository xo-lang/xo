package main

import (
	"fmt"
	"hash/adler32"
	"hash/crc64"
	"strings"
)

// The std/crc64 and std/adler32 corpora: Go's hash/crc64 and
// hash/adler32 over its own golden strings and over pseudo random
// buffers of the lengths that cross the slicing step (8), Go's slicing
// cutoff (64) and adler32's block (5552), each checked whole and
// continued at several split points.

// hsBuf is the buffer the Xo side builds with the same generator.
func hsBuf(n, seed int) []byte {
	b := make([]byte, n)
	x := seed
	for i := range b {
		x = (x*1103515245 + 12345) % 2147483648
		b[i] = byte((x >> 16) & 255)
	}
	return b
}

func hsLengths() []int {
	var ns []int
	for n := 0; n <= 40; n++ {
		ns = append(ns, n)
	}
	return append(ns, 63, 64, 65, 127, 128, 129, 255, 256, 257, 1000, 5551, 5552, 5553, 11104, 11105, 65536, 100003, 1500000)
}

const hsBufFn = `fn buf(n: Int, seed: Int) -> Bytes {
  var b = BytesBuilder.with_capacity(n)
  var x = seed
  for _ in 0..n {
    x = (x * 1103515245 + 12345) % 2147483648
    b.write_byte(UInt8((x >> 16) & 255))
  }
  b.freeze()
}

fn splits(n: Int) -> List[Int] {
  [0, 1, n / 3, n / 2, n - 1, n].filter(fn(a) { a >= 0 && a <= n })
}
`

func crc64Corpus() string {
	iso := crc64.MakeTable(crc64.ISO)
	ecma := crc64.MakeTable(crc64.ECMA)
	nvme := crc64.MakeTable(0x9a6c9329ac4bc9b5)
	var o strings.Builder
	o.WriteString(header("Go's hash/crc64"))
	o.WriteString("\nuse std/crc64\n\n" + hsBufFn)
	o.WriteString("\n/// (length, seed, ISO checksum, ECMA checksum, NVME checksum), from Go.\nfn cases() -> List[(Int, Int, Int, Int, Int)] {\n  [\n")
	for i, n := range hsLengths() {
		d := hsBuf(n, 7+i)
		fmt.Fprintf(&o, "    (%d, %d, %d, %d, %d),\n", n, 7+i, int64(crc64.Checksum(d, iso)), int64(crc64.Checksum(d, ecma)), int64(crc64.Checksum(d, nvme)))
	}
	o.WriteString("  ]\n}\n\n/// Go's crc64_test.go golden strings: (text, ISO, ECMA, NVME).\nfn golden() -> List[(Str, Int, Int, Int)] {\n  [\n")
	for _, s := range []string{"", "a", "ab", "abc", "abcd", "abcde", "abcdef", "abcdefg", "abcdefgh", "abcdefghi", "abcdefghij", "Discard medicine more than two years old.", "He who has a shady past knows that nice guys finish last.", "I wouldn't marry him with a ten foot pole.", "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave", "The days of the digital watch are numbered.  -Tom Stoppard", "Nepal premier won't resign.", "For every action there is an equal and opposite government program.", "His money is twice tainted: 'taint yours and 'taint mine.", "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977", "It's a tiny change to the code and not completely disgusting. - Bob Manchek", "size:  a.out:  bad magic", "The major problem is with sendmail.  -Mark Horton", "Give me a rock, paper and scissors and I will move the world.  CCFestoon", "If the enemy is within range, then so are you.", "It's well we cannot hear the screams/That we create in others' dreams.", "You remind me of a TV show, but that's all right: I watch it anyway.", "C is as portable as Stonehedge!!", "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley", "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule", "How can you write a big system without C++?  -Paul Glick"} {
		fmt.Fprintf(&o, "    (%s, %d, %d, %d),\n", xq(s), int64(crc64.Checksum([]byte(s), iso)), int64(crc64.Checksum([]byte(s), ecma)), int64(crc64.Checksum([]byte(s), nvme)))
	}
	o.WriteString(`  ]
}

test "checksums: Go's values" {
  for (n, seed, iso, ecma, nvme) in cases() {
    let d = buf(n, seed)
    expect(crc64.checksum(d)) == iso
    expect(crc64.checksum(d, crc64.Poly.Iso)) == iso
    expect(crc64.checksum(d, crc64.Poly.Ecma)) == ecma
    expect(crc64.checksum(d, crc64.Poly.Nvme)) == nvme
  }
}

test "checksums of Go's golden strings" {
  for (s, iso, ecma, nvme) in golden() {
    expect(crc64.checksum(s.bytes())) == iso
    expect(crc64.checksum(s.bytes(), crc64.Poly.Ecma)) == ecma
    expect(crc64.checksum(s.bytes(), crc64.Poly.Nvme)) == nvme
  }
}

test "CRC-64/NVME's check value" {
  // the catalogue's check for "123456789": 0xae8b14860a799888 (a Go uint64, so negative as an Int)
  expect(crc64.checksum(b"123456789", crc64.Poly.Nvme)) == -5869575123413395320
  expect(crc64.checksum(b"", crc64.Poly.Nvme)) == 0
}

test "update continues a checksum at any split" {
  for (n, seed, iso, ecma, nvme) in cases() {
    let d = buf(n, seed)
    for at in splits(n) {
      expect(crc64.update(crc64.checksum(d[0..at]), d[at..n])) == iso
      expect(crc64.update(crc64.checksum(d[0..at], crc64.Poly.Ecma), d[at..n], crc64.Poly.Ecma)) == ecma
      expect(crc64.update(crc64.checksum(d[0..at], crc64.Poly.Nvme), d[at..n], crc64.Poly.Nvme)) == nvme
    }
    expect(crc64.update(0, d)) == iso
  }
}

/// The bit loop of the reversed polynomial, the tables' definition.
fn bitwise(poly: Int, data: Bytes) -> Int {
  var c = ~0
  for i in 0..data.len() {
    c = c ^ Int(data[i])
    for _ in 0..8 {
      if (c & 1) != 0 {
        c = c.shr_logical(1) ^ poly
      } else {
        c = c.shr_logical(1)
      }
    }
  }
  ~c
}

test "the tables match the bit loop (every table entry is reached)" {
  let d = buf(20000, 99)
  expect(crc64.checksum(d)) == bitwise(-2882303761517117440, d)
  expect(crc64.checksum(d, crc64.Poly.Ecma)) == bitwise(-3932672073523589310, d)
  expect(crc64.checksum(d, crc64.Poly.Nvme)) == bitwise(-7319313487190308427, d)
}
`)
	return o.String()
}

func adler32Corpus() string {
	var o strings.Builder
	var ff strings.Builder
	for _, n := range []int{1, 5552, 5553, 70000, 2000000} {
		fmt.Fprintf(&ff, "  (%d, %d),\n", n, adler32.Checksum(bytesOf(0xff, n)))
	}
	o.WriteString(header("Go's hash/adler32"))
	o.WriteString("\nuse std/adler32\n\n" + hsBufFn)
	o.WriteString("\n/// (length, checksum) of that many 0xff bytes, from Go.\nconst ALLFF: List[(Int, Int)] = [\n" + ff.String() + "]\n")
	o.WriteString("\n/// (length, seed, checksum), from Go.\nfn cases() -> List[(Int, Int, Int)] {\n  [\n")
	for i, n := range hsLengths() {
		d := hsBuf(n, 7+i)
		fmt.Fprintf(&o, "    (%d, %d, %d),\n", n, 7+i, adler32.Checksum(d))
	}
	o.WriteString("  ]\n}\n\n/// Go's adler32_test.go golden strings: (text, checksum).\nfn golden() -> List[(Str, Int)] {\n  [\n")
	for _, s := range []string{"", "a", "ab", "abc", "abcd", "abcde", "abcdef", "abcdefg", "abcdefgh", "abcdefghi", "abcdefghij", "Discard medicine more than two years old.", "He who has a shady past knows that nice guys finish last.", "I wouldn't marry him with a ten foot pole.", "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave", "The days of the digital watch are numbered.  -Tom Stoppard", "Nepal premier won't resign.", "For every action there is an equal and opposite government program.", "His money is twice tainted: 'taint yours and 'taint mine.", "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977", "It's a tiny change to the code and not completely disgusting. - Bob Manchek", "size:  a.out:  bad magic", "The major problem is with sendmail.  -Mark Horton", "Give me a rock, paper and scissors and I will move the world.  CCFestoon", "If the enemy is within range, then so are you.", "It's well we cannot hear the screams/That we create in others' dreams.", "You remind me of a TV show, but that's all right: I watch it anyway.", "C is as portable as Stonehedge!!", "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley", "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule", "How can you write a big system without C++?  -Paul Glick"} {
		fmt.Fprintf(&o, "    (%s, %d),\n", xq(s), adler32.Checksum([]byte(s)))
	}
	o.WriteString(`  ]
}

test "checksums: Go's values" {
  for (n, seed, sum) in cases() {
    expect(adler32.checksum(buf(n, seed))) == sum
  }
}

test "checksums of Go's golden strings" {
  for (s, sum) in golden() {
    expect(adler32.checksum(s.bytes())) == sum
  }
  expect(adler32.checksum(b"")) == 1
}

test "a run of 0xff bytes keeps both sums in range" {
  for (n, sum) in ALLFF {
    expect(adler32.checksum(Bytes.from_hex("ff".repeat(n)) ?? b"")) == sum
  }
}

test "update continues a checksum at any split" {
  for (n, seed, sum) in cases() {
    let d = buf(n, seed)
    for at in splits(n) {
      expect(adler32.update(adler32.checksum(d[0..at]), d[at..n])) == sum
    }
    expect(adler32.update(1, d)) == sum
  }
}
`)
	return o.String()
}

func bytesOf(v byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = v
	}
	return b
}
