package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
)

// The bytes corpus: Go's bytes.Contains, HasPrefix, HasSuffix and Count as
// the oracle for Bytes.contains, starts_with, ends_with and count, on Go's
// own test cases, on the boundary cases the methods document, and on seeded
// random bytes (invalid UTF-8 included: these are bytes, not text).

// bytesCase is one (b, sub) input of the four methods.
type bytesCase struct{ b, sub []byte }

// goContainsCases are Go's bytes_test.go containsTests. Go's bytes tests
// are Copyright 2009 The Go Authors, BSD-style license.
func goContainsCases() []bytesCase {
	return []bytesCase{
		{[]byte("hello"), []byte("hel")},
		{[]byte("日本語"), []byte("日本")},
		{[]byte("hello"), []byte("Hello, world")},
		{[]byte("東京"), []byte("京東")},
	}
}

// goCountByteCases are Go's bytes_test.go TestCountByte and
// TestCountByteNoMatch windows: a buffer of one window length whose first k
// bytes are the counted byte (1 to the window) for every window length Go
// tries, so each step and tail of a 16 byte scan is counted at each
// alignment, and each window with no match at all. Go's bytes tests are
// Copyright 2009 The Go Authors, BSD-style license.
func goCountByteCases() []bytesCase {
	var cs []bytesCase
	for _, window := range []int{1, 2, 3, 4, 15, 16, 17, 31, 32, 33, 63, 64, 65, 128} {
		for k := 1; k <= window; k++ {
			b := make([]byte, window)
			for j := 0; j < k; j++ {
				b[j] = 100
			}
			cs = append(cs, bytesCase{b, []byte{100}})
		}
		b := make([]byte, window)
		for j := 0; j < window; j++ {
			b[j] = 100
		}
		cs = append(cs, bytesCase{b, []byte{0}})
	}
	return cs
}

// goEdgeCases are the boundaries the four methods document: empty values, a
// sub longer than b, one byte subs, a full match, overlapping copies (Count
// counts each once, left to right), and bytes no UTF-8 sequence can hold.
// Go's bytes tests are Copyright 2009 The Go Authors, BSD-style license.
func goEdgeCases() []bytesCase {
	return []bytesCase{
		{[]byte(""), []byte("")},
		{[]byte(""), []byte("a")},
		{[]byte("a"), []byte("")},
		{[]byte("a"), []byte("a")},
		{[]byte("a"), []byte("ab")},
		{[]byte("abc"), []byte("abc")},
		{[]byte("abc"), []byte("bc")},
		{[]byte("abc"), []byte("ab")},
		{[]byte("abc"), []byte("b")},
		{[]byte("abc"), []byte("x")},
		{[]byte("aaaaa"), []byte("aa")},
		{[]byte("aaaa"), []byte("aa")},
		{[]byte("aaa"), []byte("aa")},
		{[]byte("abababab"), []byte("abab")},
		{[]byte("\xff\xfe"), []byte("\xff")},
		{[]byte("\xff\xfe"), []byte("\xfe")},
		{[]byte("\xff\xfe"), []byte("\xff\xfe")},
		{[]byte("\xc3\xa9\xc3\xa9"), []byte("\xc3")},
		{[]byte("\x00\x00\x00"), []byte("\x00")},
		{[]byte("\x00ab\x00"), []byte("\x00")},
		{[]byte("\x00ab\x00"), []byte("\x00a")},
		{[]byte("\xed\xa0\x80\xed\xa0\x80"), []byte("\xed\xa0\x80")},
	}
}

// bytesAlphabet is the random alphabet: text, separators, NUL, multi byte
// UTF-8, and bytes no UTF-8 sequence can hold (0x80 alone, a truncated
// 0xC3, a surrogate, 0xF5).
var bytesAlphabet = []string{
	"a", "b", "z", "0", "-", "/", " ", ".", "\n", "\r\n",
	"é", "日", "\xff", "\x80", "\xc3", "\xed\xa0\x80", "\xf5", "\x00",
}

var bytesLens = []int{0, 1, 2, 3, 4, 5, 6, 8, 11, 16, 17, 23, 32, 40}

func bytesGen(r *rand.Rand, n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(bytesAlphabet[r.Intn(len(bytesAlphabet))])
	}
	return []byte(b.String())
}

// bytesRandomSub is a sub for b: empty, one piece, unrelated, longer than b,
// a prefix, a suffix or an interior piece, one byte changed in a quarter of
// the pieces.
func bytesRandomSub(r *rand.Rand, b []byte) []byte {
	switch r.Intn(8) {
	case 0:
		return nil
	case 1:
		return bytesGen(r, 1+r.Intn(3))
	case 2:
		return append(bytesGen(r, 1+r.Intn(6)), b...)
	}
	if len(b) == 0 {
		return bytesGen(r, 1+r.Intn(3))
	}
	n := 1 + r.Intn(len(b))
	start := r.Intn(len(b) - n + 1)
	sub := b[start : start+n]
	if r.Intn(4) == 0 {
		sub = append([]byte(nil), sub...)
		sub[r.Intn(len(sub))] ^= byte(1 << r.Intn(8))
	}
	return sub
}

func bytesCorpus() string {
	r := rand.New(rand.NewSource(59))
	var cs []bytesCase
	cs = append(cs, goContainsCases()...)
	cs = append(cs, goEdgeCases()...)
	cs = append(cs, goCountByteCases()...)
	for i := 0; i < 3000; i++ {
		b := bytesGen(r, bytesLens[r.Intn(len(bytesLens))])
		cs = append(cs, bytesCase{b, bytesRandomSub(r, b)})
	}

	var o strings.Builder
	o.WriteString(header("Go's bytes.Contains, HasPrefix, HasSuffix and Count on Go's bytes_test.go cases (containsTests, the TestCountByte and TestCountByteNoMatch windows), on the boundary cases the methods document, and on seeded random bytes with invalid UTF-8 (Go's bytes tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\n/// (b, sub, Go's Contains, HasPrefix, HasSuffix and Count)\nconst CASES: List[(Bytes, Bytes, Bool, Bool, Bool, Int)] = [\n")
	for _, c := range cs {
		fmt.Fprintf(&o, "  (%s, %s, %v, %v, %v, %d),\n", xbytes(c.b), xbytes(c.sub),
			bytes.Contains(c.b, c.sub), bytes.HasPrefix(c.b, c.sub), bytes.HasSuffix(c.b, c.sub), bytes.Count(c.b, c.sub))
	}
	o.WriteString("]\n")
	o.WriteString(bytesTests)
	return o.String()
}

const bytesTests = `
fn oracle_of(b: Bytes, sub: Bytes) -> Str {
  "${b.contains(sub)}\u{1f}${b.starts_with(sub)}\u{1f}${b.ends_with(sub)}\u{1f}${b.count(sub)}"
}

fn show(bad: List[(Bytes, Bytes, Str, Str)]) -> List[(Bytes, Bytes, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "contains, starts_with, ends_with and count are Go's" {
  var bad: List[(Bytes, Bytes, Str, Str)] = []
  for (b, sub, wc, ws, we, wn) in CASES {
    let want = "${wc}\u{1f}${ws}\u{1f}${we}\u{1f}${wn}"
    let got = oracle_of(b, sub)
    if got != want {
      bad.push((b, sub, got, want))
    }
  }
  expect(show(bad)) == []
}

test "the four methods on a view of a longer value" {
  // "zz" + b sliced back is a view of a longer Bytes
  var bad: List[(Bytes, Bytes, Str, Str)] = []
  for (b, sub, wc, ws, we, wn) in CASES {
    let want = "${wc}\u{1f}${ws}\u{1f}${we}\u{1f}${wn}"
    let long = b"zz" + b
    let view = long[2..long.len()]
    let got = oracle_of(view, sub)
    if got != want {
      bad.push((b, sub, got, want))
    }
  }
  expect(show(bad)) == []
}
`
