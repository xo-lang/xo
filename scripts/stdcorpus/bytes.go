package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
)

// The bytes corpus: Go's bytes package as the oracle for Bytes.contains,
// starts_with, ends_with, count, split, split_n, fields, trim, trim_prefix
// and trim_suffix, on Go's own test cases, on the boundary cases the methods
// document, and on seeded random bytes (invalid UTF-8 included: these are
// bytes, not text).

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

// bytesSplitCase is one split input with its count.
type bytesSplitCase struct {
	b, sep []byte
	n      int
}

// bytesBigN is Go's math.MaxInt / 4 in splittests: a count above any number
// of pieces a value can have.
var bytesBigN = int(^uint(0)>>1) / 4

// goBytesSplitCases are Go's bytes_test.go splittests (bytes.SplitN) on Go's
// fixtures abcd, faces, commas and dots. Go's bytes tests are Copyright 2009
// The Go Authors, BSD-style license.
func goBytesSplitCases() []bytesSplitCase {
	return []bytesSplitCase{
		{[]byte(""), []byte(""), -1},
		{[]byte("abcd"), []byte("a"), 0},
		{[]byte("abcd"), []byte(""), 2},
		{[]byte("abcd"), []byte("a"), -1},
		{[]byte("abcd"), []byte("z"), -1},
		{[]byte("abcd"), []byte(""), -1},
		{[]byte("1,2,3,4"), []byte(","), -1},
		{[]byte("1....2....3....4"), []byte("..."), -1},
		{[]byte("☺☻☹"), []byte("☹"), -1},
		{[]byte("☺☻☹"), []byte("~"), -1},
		{[]byte("☺☻☹"), []byte(""), -1},
		{[]byte("1 2 3 4"), []byte(" "), 3},
		{[]byte("1 2"), []byte(" "), 3},
		{[]byte("123"), []byte(""), 2},
		{[]byte("123"), []byte(""), 17},
		{[]byte("bT"), []byte("T"), bytesBigN},
		{[]byte("\xff-\xff"), []byte(""), -1},
		{[]byte("\xff-\xff"), []byte("-"), -1},
	}
}

// goBytesFieldsCases are Go's bytes_test.go fieldstests. Go's bytes tests are
// Copyright 2009 The Go Authors, BSD-style license.
func goBytesFieldsCases() [][]byte {
	return [][]byte{
		[]byte(""),
		[]byte(" "),
		[]byte(" \t "),
		[]byte("  abc  "),
		[]byte("1 2 3 4"),
		[]byte("1  2  3  4"),
		[]byte("1\t\t2\t\t3\t4"),
		[]byte("1\u20002\u20013\u20024"),
		[]byte("\u2000\u2001\u2002"),
		[]byte("\n™\t™\n"),
		[]byte("☺☻☹"),
	}
}

// goTrimSpaceCases are Go's bytes_test.go trimSpaceTests, in the order Go
// has them (its space is every kind of white space). Go's bytes tests are
// Copyright 2009 The Go Authors, BSD-style license.
func goTrimSpaceCases() [][]byte {
	const space = "\t\v\r\f\n\u0085\u00a0\u2000\u3000"
	return [][]byte{
		[]byte(""),
		[]byte("  a"),
		[]byte("b  "),
		[]byte("abc"),
		[]byte(space + "abc" + space),
		[]byte(" "),
		[]byte("\u3000 "),
		[]byte(" \u3000"),
		[]byte(" \t\r\n \t\t\r\r\n\n "),
		[]byte(" \t\r\n x\t\t\r\r\n\n "),
		[]byte(" \u2000\t\r\n x\t\t\r\r\ny\n \u3000"),
		[]byte("1 \t\r\n2"),
		[]byte(" x\x80"),
		[]byte(" x\xc0"),
		[]byte("x \xc0\xc0 "),
		[]byte("x \xc0"),
		[]byte("x \xc0 "),
		[]byte("x \xc0\xc0 "),
		[]byte("x ☺\xc0\xc0 "),
		[]byte("x ☺ "),
	}
}

// goTrimAffixCases are the TrimPrefix and TrimSuffix cases of Go's
// bytes_test.go trimTests. Go's bytes tests are Copyright 2009 The Go
// Authors, BSD-style license.
func goTrimAffixCases() [][2][]byte {
	return [][2][]byte{
		{[]byte("aabb"), []byte("a")},
		{[]byte("aabb"), []byte("b")},
		{[]byte("aabb"), []byte("aabb")},
		{[]byte("aabb"), []byte("")},
		{[]byte(""), []byte("a")},
	}
}

// bytesSpaceAlphabet is the white space of both kinds (ASCII and the
// non-ASCII runes unicode.IsSpace holds), text, NUL and bytes no UTF-8
// sequence can hold.
var bytesSpaceAlphabet = []string{
	" ", "\t", "\n", "\v", "\f", "\r", "\u0085", "\u00a0", "\u2000", "\u3000",
	"a", "b", "é", "日", "\xff", "\x80", "\xc3", "\xed\xa0\x80", "\x00",
}

// bytesRandomSpace is up to 8 pieces of bytesSpaceAlphabet, so runs of white
// space, none at all, and a run at either end all occur.
func bytesRandomSpace(r *rand.Rand) []byte {
	var b strings.Builder
	for i := 0; i < r.Intn(9); i++ {
		b.WriteString(bytesSpaceAlphabet[r.Intn(len(bytesSpaceAlphabet))])
	}
	return []byte(b.String())
}

// bytesRandomSplit is a random split input: an empty, one piece, unrelated,
// longer or interior separator, and a count of every kind.
func bytesRandomSplit(r *rand.Rand) bytesSplitCase {
	b := bytesGen(r, bytesLens[r.Intn(len(bytesLens))])
	var sep []byte
	switch r.Intn(5) {
	case 0:
		sep = nil
	case 1, 2:
		sep = bytesGen(r, 1+r.Intn(2))
	default:
		sep = bytesRandomSub(r, b)
	}
	n := -1
	switch r.Intn(6) {
	case 0:
		n = 0
	case 1:
		n = 1
	case 2:
		n = 2
	case 3:
		n = 3
	case 4:
		n = 1 << 40
	}
	return bytesSplitCase{b, sep, n}
}

// bytesRandomAffix is a random affix input: an empty p, a prefix, a suffix,
// an unrelated piece or a p longer than b.
func bytesRandomAffix(r *rand.Rand) [2][]byte {
	b := bytesGen(r, bytesLens[r.Intn(len(bytesLens))])
	switch r.Intn(6) {
	case 0:
		return [2][]byte{b, nil}
	case 1:
		return [2][]byte{b, bytesGen(r, 1+r.Intn(3))}
	case 2, 3:
		if len(b) > 0 {
			return [2][]byte{b, b[:1+r.Intn(len(b))]}
		}
	case 4:
		if len(b) > 0 {
			return [2][]byte{b, b[len(b)-(1+r.Intn(len(b))):]}
		}
	}
	return [2][]byte{b, append(bytesGen(r, 1+r.Intn(2)), b...)}
}

// xbytesList is parts as an Xo List[Bytes] literal.
func xbytesList(parts [][]byte) string {
	var b strings.Builder
	b.WriteString("[")
	for i, p := range parts {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(xbytes(p))
	}
	b.WriteString("]")
	return b.String()
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

	// split (its own seed, so the section above keeps its inputs)
	sp := goBytesSplitCases()
	rsp := rand.New(rand.NewSource(61))
	for i := 0; i < 1200; i++ {
		sp = append(sp, bytesRandomSplit(rsp))
	}
	o.WriteString("\n/// (b, sep, n, Go's SplitN)\nconst SPLIT_CASES: List[(Bytes, Bytes, Int, List[Bytes])] = [\n")
	for _, c := range sp {
		fmt.Fprintf(&o, "  (%s, %s, %d, %s),\n", xbytes(c.b), xbytes(c.sep), c.n, xbytesList(bytes.SplitN(c.b, c.sep, c.n)))
	}
	o.WriteString("]\n")

	// fields
	fd := goBytesFieldsCases()
	rfd := rand.New(rand.NewSource(67))
	for i := 0; i < 1200; i++ {
		fd = append(fd, bytesRandomSpace(rfd))
	}
	o.WriteString("\n/// (b, Go's Fields)\nconst FIELDS_CASES: List[(Bytes, List[Bytes])] = [\n")
	for _, c := range fd {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(c), xbytesList(bytes.Fields(c)))
	}
	o.WriteString("]\n")

	// trim (TrimSpace)
	tr := goTrimSpaceCases()
	rtr := rand.New(rand.NewSource(71))
	for i := 0; i < 1200; i++ {
		tr = append(tr, bytesRandomSpace(rtr))
	}
	o.WriteString("\n/// (b, Go's TrimSpace)\nconst TRIM_CASES: List[(Bytes, Bytes)] = [\n")
	for _, c := range tr {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(c), xbytes(bytes.TrimSpace(c)))
	}
	o.WriteString("]\n")

	// trim_prefix and trim_suffix
	af := goTrimAffixCases()
	raf := rand.New(rand.NewSource(73))
	for i := 0; i < 1200; i++ {
		af = append(af, bytesRandomAffix(raf))
	}
	o.WriteString("\n/// (b, p, Go's TrimPrefix, Go's TrimSuffix)\nconst AFFIX_CASES: List[(Bytes, Bytes, Bytes, Bytes)] = [\n")
	for _, c := range af {
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xbytes(c[0]), xbytes(c[1]), xbytes(bytes.TrimPrefix(c[0], c[1])), xbytes(bytes.TrimSuffix(c[0], c[1])))
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

test "split_n is Go's SplitN" {
  for (b, sep, n, want) in SPLIT_CASES {
    expect((b, sep, n, b.split_n(sep, n))) == (b, sep, n, want)
  }
}

test "split is SplitN with every piece" {
  for (b, sep, n, want) in SPLIT_CASES {
    if n < 0 {
      expect((b, sep, b.split(sep))) == (b, sep, want)
    }
  }
}

test "fields is Go's Fields" {
  for (b, want) in FIELDS_CASES {
    expect((b, b.fields())) == (b, want)
  }
}

test "trim is Go's TrimSpace" {
  for (b, want) in TRIM_CASES {
    expect((b, b.trim())) == (b, want)
  }
}

test "trim_prefix and trim_suffix are Go's" {
  for (b, p, wp, ws) in AFFIX_CASES {
    expect((b, p, b.trim_prefix(p), b.trim_suffix(p))) == (b, p, wp, ws)
  }
}

test "the split and trim family on a view of a longer value" {
  // "zz" + b sliced back is a view of a longer Bytes: the pieces are views
  // of a view
  for (b, sep, n, want) in SPLIT_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    expect((b, sep, n, view.split_n(sep, n))) == (b, sep, n, want)
  }
  for (b, want) in TRIM_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    expect((b, view.trim())) == (b, want)
  }
  for (b, want) in FIELDS_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    expect((b, view.fields())) == (b, want)
  }
}
`
