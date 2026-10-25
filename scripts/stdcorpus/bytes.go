package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
)

// The bytes corpus: Go's bytes package as the oracle for Bytes.contains,
// starts_with, ends_with, count, split, split_n, fields, trim, trim_prefix,
// trim_suffix, replace, replace_n, to_lower, to_upper, equal_fold and
// repeat, on Go's own test cases, on the boundary cases the methods
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

// bytesReplaceCase is one replace input: the bytes, old, new and the count
// (n below 0: every copy).
type bytesReplaceCase struct {
	b, old, new []byte
	n           int
}

// goBytesReplaceCases are Go's bytes_test.go ReplaceTests, which its
// TestReplace runs through bytes.Replace and, for n -1, bytes.ReplaceAll.
// Go's bytes tests are Copyright 2009 The Go Authors, BSD-style license.
func goBytesReplaceCases() []bytesReplaceCase {
	return []bytesReplaceCase{
		{[]byte("hello"), []byte("l"), []byte("L"), 0},
		{[]byte("hello"), []byte("l"), []byte("L"), -1},
		{[]byte("hello"), []byte("x"), []byte("X"), -1},
		{[]byte(""), []byte("x"), []byte("X"), -1},
		{[]byte("radar"), []byte("r"), []byte("<r>"), -1},
		{[]byte(""), []byte(""), []byte("<>"), -1},
		{[]byte("banana"), []byte("a"), []byte("<>"), -1},
		{[]byte("banana"), []byte("a"), []byte("<>"), 1},
		{[]byte("banana"), []byte("a"), []byte("<>"), 1000},
		{[]byte("banana"), []byte("an"), []byte("<>"), -1},
		{[]byte("banana"), []byte("ana"), []byte("<>"), -1},
		{[]byte("banana"), []byte(""), []byte("<>"), -1},
		{[]byte("banana"), []byte(""), []byte("<>"), 10},
		{[]byte("banana"), []byte(""), []byte("<>"), 6},
		{[]byte("banana"), []byte(""), []byte("<>"), 5},
		{[]byte("banana"), []byte(""), []byte("<>"), 1},
		{[]byte("banana"), []byte("a"), []byte("a"), -1},
		{[]byte("banana"), []byte("a"), []byte("a"), 1},
		{[]byte("☺☻☹"), []byte(""), []byte("<>"), -1},
	}
}

// goBytesCaseCases are the inputs of Go's bytes_test.go upperTests and
// lowerTests, which its TestToUpper and TestToLower run through
// bytes.ToUpper and bytes.ToLower. Go's bytes tests are Copyright 2009 The
// Go Authors, BSD-style license.
func goBytesCaseCases() [][]byte {
	return [][]byte{
		// upperTests
		[]byte(""),
		[]byte("ONLYUPPER"),
		[]byte("abc"),
		[]byte("AbC123"),
		[]byte("azAZ09_"),
		[]byte("longStrinGwitHmixofsmaLLandcAps"),
		[]byte("long\u0250string\u0250with\u0250nonascii\u2C6Fchars"),
		[]byte("\u0250\u0250\u0250\u0250\u0250"),
		[]byte("a\u0080\U0010FFFF"),
		// lowerTests
		[]byte("abc"),
		[]byte("AbC123"),
		[]byte("azAZ09_"),
		[]byte("longStrinGwitHmixofsmaLLandcAps"),
		[]byte("LONG\u2C6FSTRING\u2C6FWITH\u2C6FNONASCII\u2C6FCHARS"),
		[]byte("\u2C6D\u2C6D\u2C6D\u2C6D\u2C6D"),
		[]byte("A\u0080\U0010FFFF"),
	}
}

// caseBoundaryCases are the inputs the ASCII fold's word loop needs: every
// length below, at and just past its words (0, 7, 8, 9, 15, 16 and 17), a
// byte at or above 0x80 at each position of an eight byte and a sixteen
// byte word, and the bytes next to each letter range (0x40, 0x5b, 0x60 and
// 0x7b around A-Z and a-z), which a range test on the wrong side takes for
// letters. The expectations are Go's bytes.ToLower and bytes.ToUpper, as for
// the rows above.
func caseBoundaryCases() [][]byte {
	var cs [][]byte
	for _, s := range []string{"a", "A", "ab", "Ab", "aB"} {
		for _, n := range []int{0, 1, 7, 8, 9, 15, 16, 17, 23, 24, 25} {
			cs = append(cs, []byte(strings.Repeat(s[:1], n)))
		}
	}
	for _, w := range []string{"abcdefgh", "ABCDEFGH", "abcdefghijklmnop", "ABCDEFGHIJKLMNOP"} {
		cs = append(cs, []byte(w))
		for k := 0; k < len(w); k++ {
			for _, hi := range []byte{0x80, 0xc3, 0xff} {
				b := []byte(w)
				b[k] = hi
				cs = append(cs, b)
			}
		}
	}
	for _, s := range []string{
		"@AZ[", "@az[", "[A", "Z@", "Z[", "A@", "z`", "z{", "`a", "{a", "Z[\\]", "z{|}",
		"@ABCDEFGHIJKLMNOPQRSTUVWXYZ[", "`abcdefghijklmnopqrstuvwxyz{",
		"@ABCDEFG[A@", "`abcdefg{`a", "[Z@A][", "{z`a{",
		"Aa@[{@`a", "ABCDEFGHIJKLMNOPqrstuvwxyz@[`{",
	} {
		cs = append(cs, []byte(s))
	}
	return cs
}

// goBytesEqualFoldCases are the pairs of Go's bytes_test.go
// EqualFoldTests. Go's bytes tests are Copyright 2009 The Go Authors,
// BSD-style license.
func goBytesEqualFoldCases() [][2][]byte {
	return [][2][]byte{
		{[]byte("abc"), []byte("abc")},
		{[]byte("ABcd"), []byte("ABcd")},
		{[]byte("123abc"), []byte("123ABC")},
		{[]byte("αβδ"), []byte("ΑΒΔ")},
		{[]byte("abc"), []byte("xyz")},
		{[]byte("abc"), []byte("XYZ")},
		{[]byte("abcdefghijk"), []byte("abcdefghijX")},
		{[]byte("abcdefghijk"), []byte("abcdefghij\u212A")},
		{[]byte("abcdefghijK"), []byte("abcdefghij\u212A")},
		{[]byte("abcdefghijkz"), []byte("abcdefghij\u212Ay")},
		{[]byte("abcdefghijKz"), []byte("abcdefghij\u212Ay")},
	}
}

// bytesRepeatCase is one repeat input: the bytes and the count.
type bytesRepeatCase struct {
	b []byte
	n int
}

// goBytesRepeatCases are Go's bytes_test.go RepeatTests. Its two entries
// over Go's internal chunk limit (65536 NUL bytes, and a 131 KB text
// repeated twice) would be a 400 KB corpus literal and test Go's chunk
// buffer, which the native runtime does not have; a 40 byte NUL run stands
// in for the first. Go's bytes tests are Copyright 2009 The Go Authors,
// BSD-style license.
func goBytesRepeatCases() []bytesRepeatCase {
	return []bytesRepeatCase{
		{[]byte(""), 0},
		{[]byte(""), 1},
		{[]byte(""), 2},
		{[]byte("-"), 0},
		{[]byte("-"), 1},
		{[]byte("-"), 10},
		{[]byte("abc "), 3},
		{[]byte("\x00"), 40},
	}
}

// bytesCaseAlphabet is the alphabet of the case and fold sections: ASCII
// letters, a digit, text, the widths that change under case mapping
// (U+0130 down to i, U+0250 up to U+2C6F, U+00DF up to U+1E9E), the
// SimpleFold orbit members (k, K, the Kelvin sign U+212A; sigma's three
// forms), and bytes no UTF-8 sequence can hold.
var bytesCaseAlphabet = []string{
	"a", "b", "k", "s", "z", "A", "B", "K", "S", "0", " ", "-", "\n",
	"é", "É", "日", "σ", "ς", "Σ", "ß", "İ", "ı", "ɐ", "Ɐ", "Ɑ", "K", "Å", "\u212A",
	"\u1E9E", "\u0130", "\xff", "\x80", "\xc3", "\xed\xa0\x80",
}

func bytesCaseGen(r *rand.Rand, n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(bytesCaseAlphabet[r.Intn(len(bytesCaseAlphabet))])
	}
	return []byte(b.String())
}

// bytesRandomReplace is a random replace input: old is empty, one piece,
// unrelated, longer than b or a piece of it, new is empty or short, and the
// count is every kind.
func bytesRandomReplace(r *rand.Rand) bytesReplaceCase {
	b := bytesGen(r, bytesLens[r.Intn(len(bytesLens))])
	old := bytesRandomSub(r, b)
	if r.Intn(3) == 0 {
		old = nil
	}
	new := bytesGen(r, r.Intn(4))
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
		n = 1000
	}
	return bytesReplaceCase{b, old, new, n}
}

// bytesRandomCase is a random byte text over bytesCaseAlphabet, for
// to_lower and to_upper.
func bytesRandomCase(r *rand.Rand) []byte {
	return bytesCaseGen(r, r.Intn(9))
}

// bytesRandomFold is a random fold input: a case flipped copy (a fold
// match), an unrelated text, or a copy with one byte changed.
func bytesRandomFold(r *rand.Rand) [2][]byte {
	a := bytesCaseGen(r, r.Intn(8))
	switch r.Intn(3) {
	case 0:
		if r.Intn(2) == 0 {
			return [2][]byte{a, bytes.ToUpper(append([]byte{}, a...))}
		}
		return [2][]byte{a, bytes.ToLower(append([]byte{}, a...))}
	case 1:
		return [2][]byte{a, bytesCaseGen(r, r.Intn(8))}
	}
	b := append([]byte{}, a...)
	if len(b) > 0 {
		b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
	}
	return [2][]byte{a, b}
}

// bytesRandomRepeat is a random repeat input: up to 6 bytes and a count of
// 0 to 5.
func bytesRandomRepeat(r *rand.Rand) bytesRepeatCase {
	return bytesRepeatCase{bytesGen(r, r.Intn(7)), r.Intn(6)}
}

// bytesAnyCase is one (b, chars) input of find_any, rfind_any and
// contains_any. chars is text (a Str), as Go's bytes.IndexAny takes a
// string.
type bytesAnyCase struct {
	b     []byte
	chars []byte
}

// bytesRuneCase is one (b, r) input of contains_rune.
type bytesRuneCase struct {
	b []byte
	r rune
}

// bytesCutCase is one (b, sep) input of cut, cut_prefix and cut_suffix.
type bytesCutCase struct {
	b, sep []byte
}

// bytesSplitAfterCase is one input of split_after and split_after_n.
type bytesSplitAfterCase struct {
	b, sep []byte
	n      int
}

// goBytesAnyCases are the inputs of Go's bytes_test.go indexAnyTests and
// lastIndexAnyTests, unioned (they overlap). Go's bytes tests are Copyright
// 2009 The Go Authors, BSD-style license.
func goBytesAnyCases() []bytesAnyCase {
	dots := "1....2....3....4"
	in := [][2]string{
		{"", ""}, {"", "a"}, {"", "abc"}, {"a", ""}, {"a", "a"},
		{"\x80", "\xffb"}, {"aaa", "a"}, {"abc", "xyz"}, {"abc", "xcz"}, {"abc", "ab"},
		{"ab☺c", "x☺yz"}, {"a☺b☻c☹d", "cx"}, {"a☺b☻c☹d", "uvw☻xyz"},
		{"aRegExp*", ".(|)*+?^$[]"}, {"a.RegExp*", ".(|)*+?^$[]"},
		{dots + dots + dots, " "},
		{"012abcba210", "\xffb"}, {"012\x80bcb\x80210", "\xffb"},
		{"0123456\xcf\x80abc", "\xcfb\x80"},
	}
	var out []bytesAnyCase
	for _, c := range in {
		out = append(out, bytesAnyCase{[]byte(c[0]), []byte(c[1])})
	}
	return out
}

// goBytesRuneCases are Go's bytes_test.go ContainsRuneTests. Go's bytes
// tests are Copyright 2009 The Go Authors, BSD-style license.
func goBytesRuneCases() []bytesRuneCase {
	return []bytesRuneCase{
		{[]byte(""), 'a'},
		{[]byte("a"), 'a'},
		{[]byte("aaa"), 'a'},
		{[]byte("abc"), 'y'},
		{[]byte("abc"), 'c'},
		{[]byte("a☺b☻c☹d"), 'x'},
		{[]byte("a☺b☻c☹d"), '☻'},
		{[]byte("aRegExp*"), '*'},
	}
}

// goBytesCutCases are Go's bytes_test.go cutTests. Go's bytes tests are
// Copyright 2009 The Go Authors, BSD-style license.
func goBytesCutCases() []bytesCutCase {
	in := [][2]string{
		{"abc", "b"}, {"abc", "a"}, {"abc", "c"}, {"abc", "abc"},
		{"abc", ""}, {"abc", "d"}, {"", "d"}, {"", ""},
	}
	var out []bytesCutCase
	for _, c := range in {
		out = append(out, bytesCutCase{[]byte(c[0]), []byte(c[1])})
	}
	return out
}

// bytesCutBoundaryCases are cutBoundaryCases as Bytes: the separator
// lengths and positions where the separator compare's word path switches
// (2026-10-25).
func bytesCutBoundaryCases() []bytesCutCase {
	var out []bytesCutCase
	for _, c := range cutBoundaryCases() {
		out = append(out, bytesCutCase{[]byte(c.s), []byte(c.sep)})
	}
	return out
}

// goBytesCutPrefixCases are Go's bytes_test.go cutPrefixTests, and
// goBytesCutSuffixCases its cutSuffixTests. Go's bytes tests are Copyright
// 2009 The Go Authors, BSD-style license.
func goBytesCutPrefixCases() []bytesCutCase {
	in := [][2]string{
		{"abc", "a"}, {"abc", "abc"}, {"abc", ""}, {"abc", "d"}, {"", "d"}, {"", ""},
	}
	var out []bytesCutCase
	for _, c := range in {
		out = append(out, bytesCutCase{[]byte(c[0]), []byte(c[1])})
	}
	return out
}

func goBytesCutSuffixCases() []bytesCutCase {
	in := [][2]string{
		{"abc", "bc"}, {"abc", "abc"}, {"abc", ""}, {"abc", "d"}, {"", "d"}, {"", ""},
	}
	var out []bytesCutCase
	for _, c := range in {
		out = append(out, bytesCutCase{[]byte(c[0]), []byte(c[1])})
	}
	return out
}

// goBytesSplitAfterCases are Go's bytes_test.go splitaftertests on its
// fixtures. Go's bytes tests are Copyright 2009 The Go Authors, BSD-style
// license.
func goBytesSplitAfterCases() []bytesSplitAfterCase {
	abcd, faces, commas, dots := "abcd", "☺☻☹", "1,2,3,4", "1....2....3....4"
	in := []bytesSplitAfterCase{
		{[]byte(abcd), []byte("a"), -1},
		{[]byte(abcd), []byte("z"), -1},
		{[]byte(abcd), []byte(""), -1},
		{[]byte(commas), []byte(","), -1},
		{[]byte(dots), []byte("..."), -1},
		{[]byte(faces), []byte("☹"), -1},
		{[]byte(faces), []byte("~"), -1},
		{[]byte(faces), []byte(""), -1},
		{[]byte("1 2 3 4"), []byte(" "), 3},
		{[]byte("1 2 3"), []byte(" "), 3},
		{[]byte("1 2"), []byte(" "), 3},
		{[]byte("123"), []byte(""), 2},
		{[]byte("123"), []byte(""), 17},
	}
	return in
}

// bytesRandomAny is a random (b, chars): chars is empty, one byte, text
// holding a multi byte rune and an invalid byte, or a piece of b (which can
// start inside a sequence, so the set holds an invalid byte).
func bytesRandomAny(r *rand.Rand) bytesAnyCase {
	b := bytesGen(r, bytesLens[r.Intn(len(bytesLens))])
	var chars []byte
	switch r.Intn(5) {
	case 0:
		chars = nil
	case 1:
		chars = bytesGen(r, 1)
	case 2:
		chars = []byte("aπ☺\xffb")
	default:
		if len(b) > 0 {
			chars = b[r.Intn(len(b)):]
			if len(chars) > 3 {
				chars = chars[:1+r.Intn(3)]
			}
		} else {
			chars = bytesGen(r, 1)
		}
	}
	return bytesAnyCase{b, chars}
}

// bytesRandomCut is a random (b, sep): sep empty, one byte, unrelated, a
// piece of b, or longer than b.
func bytesRandomCut(r *rand.Rand) bytesCutCase {
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
	return bytesCutCase{b, sep}
}

// bytesRandomRune is a random (b, r): r is ASCII, a two or three byte rune,
// U+FFFD or the last code point.
func bytesRandomRune(r *rand.Rand) bytesRuneCase {
	b := bytesGen(r, bytesLens[r.Intn(len(bytesLens))])
	rs := []rune{'a', 'A', '0', ':', 'é', 'π', '☺', '☹', 0xFFFD, 0x10FFFF}
	return bytesRuneCase{b, rs[r.Intn(len(rs))]}
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
	o.WriteString(header("Go's bytes.Contains, HasPrefix, HasSuffix, Count, SplitN, SplitAfter, Fields, TrimSpace, TrimPrefix, TrimSuffix, Replace, ToLower, ToUpper, EqualFold, Repeat, IndexAny, LastIndexAny, ContainsAny, ContainsRune, Cut, CutPrefix and CutSuffix on Go's bytes_test.go cases (containsTests, the TestCountByte and TestCountByteNoMatch windows, splittests, splitaftertests, fieldstests, trimSpaceTests, trimTests, ReplaceTests, upperTests, lowerTests, EqualFoldTests, RepeatTests, indexAnyTests, lastIndexAnyTests, ContainsRuneTests, cutTests, cutPrefixTests, cutSuffixTests), on the boundary cases the methods document, and on seeded random bytes with invalid UTF-8 (Go's bytes tests are Copyright 2009 The Go Authors, BSD-style license)"))
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

	// replace and replace_n (its own seed, so the sections above keep
	// their inputs)
	rp := goBytesReplaceCases()
	rrp := rand.New(rand.NewSource(79))
	for i := 0; i < 1500; i++ {
		rp = append(rp, bytesRandomReplace(rrp))
	}
	o.WriteString("\n/// (b, old, new, n, Go's Replace with n; n below 0 is ReplaceAll)\nconst REPLACE_CASES: List[(Bytes, Bytes, Bytes, Int, Bytes)] = [\n")
	for _, c := range rp {
		fmt.Fprintf(&o, "  (%s, %s, %s, %d, %s),\n", xbytes(c.b), xbytes(c.old), xbytes(c.new), c.n, xbytes(bytes.Replace(c.b, c.old, c.new, c.n)))
	}
	o.WriteString("]\n")

	// to_lower and to_upper
	cm := goBytesCaseCases()
	cm = append(cm, caseBoundaryCases()...)
	rcm := rand.New(rand.NewSource(83))
	for i := 0; i < 1200; i++ {
		cm = append(cm, bytesRandomCase(rcm))
	}
	o.WriteString("\n/// (b, Go's ToLower, Go's ToUpper)\nconst CASE_CASES: List[(Bytes, Bytes, Bytes)] = [\n")
	for _, c := range cm {
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(c), xbytes(bytes.ToLower(c)), xbytes(bytes.ToUpper(c)))
	}
	o.WriteString("]\n")

	// equal_fold
	ef := goBytesEqualFoldCases()
	ref := rand.New(rand.NewSource(89))
	for i := 0; i < 1500; i++ {
		ef = append(ef, bytesRandomFold(ref))
	}
	o.WriteString("\n/// (a, b, Go's EqualFold)\nconst FOLD_CASES: List[(Bytes, Bytes, Bool)] = [\n")
	for _, c := range ef {
		fmt.Fprintf(&o, "  (%s, %s, %v),\n", xbytes(c[0]), xbytes(c[1]), bytes.EqualFold(c[0], c[1]))
	}
	o.WriteString("]\n")

	// repeat
	rpt := goBytesRepeatCases()
	rrpt := rand.New(rand.NewSource(97))
	for i := 0; i < 1200; i++ {
		rpt = append(rpt, bytesRandomRepeat(rrpt))
	}
	o.WriteString("\n/// (b, n, Go's Repeat)\nconst REPEAT_CASES: List[(Bytes, Int, Bytes)] = [\n")
	for _, c := range rpt {
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xbytes(c.b), c.n, xbytes(bytes.Repeat(c.b, c.n)))
	}
	o.WriteString("]\n")

	// find_any, rfind_any and contains_any (its own seed, so the sections
	// above keep their inputs)
	an := goBytesAnyCases()
	ran := rand.New(rand.NewSource(101))
	for i := 0; i < 1500; i++ {
		an = append(an, bytesRandomAny(ran))
	}
	o.WriteString("\n/// (b, chars, Go's IndexAny, LastIndexAny and ContainsAny joined by U+001F; -1 for no match)\nconst ANY_CASES: List[(Bytes, Str, Str)] = [\n")
	for _, c := range an {
		chars := textOf(string(c.chars))
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(c.b), xq(chars),
			xq(fmt.Sprintf("%d\x1f%d\x1f%v", bytes.IndexAny(c.b, chars), bytes.LastIndexAny(c.b, chars), bytes.ContainsAny(c.b, chars))))
	}
	o.WriteString("]\n")

	// contains_rune
	rn := goBytesRuneCases()
	rrn := rand.New(rand.NewSource(103))
	for i := 0; i < 1200; i++ {
		rn = append(rn, bytesRandomRune(rrn))
	}
	o.WriteString("\n/// (b, r, Go's ContainsRune)\nconst RUNE_CASES: List[(Bytes, Rune, Bool)] = [\n")
	for _, c := range rn {
		fmt.Fprintf(&o, "  (%s, %s, %v),\n", xbytes(c.b), xr(c.r), bytes.ContainsRune(c.b, c.r))
	}
	o.WriteString("]\n")

	// cut, cut_prefix and cut_suffix
	cu := append(goBytesCutCases(), bytesCutBoundaryCases()...)
	rcu := rand.New(rand.NewSource(107))
	for i := 0; i < 1200; i++ {
		cu = append(cu, bytesRandomCut(rcu))
	}
	o.WriteString("\n/// (b, sep, whether Go's Cut finds it, its before and after)\nconst CUT_CASES: List[(Bytes, Bytes, Bool, Bytes, Bytes)] = [\n")
	for _, c := range cu {
		before, after, ok := bytes.Cut(c.b, c.sep)
		if !ok {
			before, after = []byte{}, []byte{}
		}
		fmt.Fprintf(&o, "  (%s, %s, %v, %s, %s),\n", xbytes(c.b), xbytes(c.sep), ok, xbytes(before), xbytes(after))
	}
	o.WriteString("]\n")

	cp := append(goBytesCutPrefixCases(), bytesCutBoundaryCases()...)
	rcp := rand.New(rand.NewSource(109))
	for i := 0; i < 1200; i++ {
		cp = append(cp, bytesRandomCut(rcp))
	}
	o.WriteString("\n/// (b, prefix, whether Go's CutPrefix finds it, its after)\nconst CUTP_CASES: List[(Bytes, Bytes, Bool, Bytes)] = [\n")
	for _, c := range cp {
		after, ok := bytes.CutPrefix(c.b, c.sep)
		if !ok {
			after = []byte{}
		}
		fmt.Fprintf(&o, "  (%s, %s, %v, %s),\n", xbytes(c.b), xbytes(c.sep), ok, xbytes(after))
	}
	o.WriteString("]\n")

	csx := append(goBytesCutSuffixCases(), bytesCutBoundaryCases()...)
	rcsx := rand.New(rand.NewSource(113))
	for i := 0; i < 1200; i++ {
		csx = append(csx, bytesRandomCut(rcsx))
	}
	o.WriteString("\n/// (b, suffix, whether Go's CutSuffix finds it, its before)\nconst CUTS_CASES: List[(Bytes, Bytes, Bool, Bytes)] = [\n")
	for _, c := range csx {
		before, ok := bytes.CutSuffix(c.b, c.sep)
		if !ok {
			before = []byte{}
		}
		fmt.Fprintf(&o, "  (%s, %s, %v, %s),\n", xbytes(c.b), xbytes(c.sep), ok, xbytes(before))
	}
	o.WriteString("]\n")

	// split_after and split_after_n
	sa := goBytesSplitAfterCases()
	rsa := rand.New(rand.NewSource(127))
	for i := 0; i < 1200; i++ {
		c := bytesRandomSplit(rsa)
		sa = append(sa, bytesSplitAfterCase{c.b, c.sep, c.n})
	}
	o.WriteString("\n/// (b, sep, n, Go's SplitAfterN)\nconst SPLITAFTER_CASES: List[(Bytes, Bytes, Int, List[Bytes])] = [\n")
	for _, c := range sa {
		fmt.Fprintf(&o, "  (%s, %s, %d, %s),\n", xbytes(c.b), xbytes(c.sep), c.n, xbytesList(bytes.SplitAfterN(c.b, c.sep, c.n)))
	}
	o.WriteString("]\n")

	o.WriteString(bytesTests)
	o.WriteString(byteswhereCorpus())
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


test "replace and replace_n are Go's" {
  for (b, old, new, n, want) in REPLACE_CASES {
    expect((b, old, new, n, b.replace_n(old, new, n))) == (b, old, new, n, want)
    if n < 0 {
      // ReplaceAll is replace_n with every copy
      expect((b, old, new, b.replace(old, new))) == (b, old, new, want)
    }
  }
}

test "to_lower and to_upper are Go's" {
  for (b, low, up) in CASE_CASES {
    expect((b, b.to_lower(), b.to_upper())) == (b, low, up)
  }
}

test "equal_fold is Go's EqualFold" {
  for (a, b, want) in FOLD_CASES {
    expect((a, b, a.equal_fold(b), b.equal_fold(a))) == (a, b, want, want)
  }
}

test "repeat is Go's Repeat" {
  for (b, n, want) in REPEAT_CASES {
    expect((b, n, b.repeat(n))) == (b, n, want)
  }
}

test "the transforms on a view of a longer value" {
  for (b, old, new, n, want) in REPLACE_CASES {
    let lb = b"zz" + b
    let lo = b"zz" + old
    let ln = b"zz" + new
    let vb = lb[2..lb.len()]
    let vo = lo[2..lo.len()]
    let vn = ln[2..ln.len()]
    expect((b, old, new, n, vb.replace_n(vo, vn, n))) == (b, old, new, n, want)
  }
  for (b, low, up) in CASE_CASES {
    let lb = b"zz" + b
    let vb = lb[2..lb.len()]
    expect((b, vb.to_lower(), vb.to_upper())) == (b, low, up)
  }
  for (a, b, want) in FOLD_CASES {
    let la = b"zz" + a
    let lb = b"zz" + b
    let va = la[2..la.len()]
    let vb = lb[2..lb.len()]
    expect((a, b, va.equal_fold(vb))) == (a, b, want)
  }
  for (b, n, want) in REPEAT_CASES {
    let lb = b"zz" + b
    let vb = lb[2..lb.len()]
    expect((b, n, vb.repeat(n))) == (b, n, want)
  }
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

fn any_of(b: Bytes, chars: Str) -> Str {
  "${b.find_any(chars) ?? -1}\u{1f}${b.rfind_any(chars) ?? -1}\u{1f}${b.contains_any(chars)}"
}

test "find_any, rfind_any and contains_any are Go's" {
  for (b, chars, want) in ANY_CASES {
    expect((b, chars, any_of(b, chars))) == (b, chars, want)
  }
}

test "contains_rune is Go's" {
  for (b, r, want) in RUNE_CASES {
    expect((b, r, b.contains_rune(r))) == (b, r, want)
  }
}

test "cut is Go's Cut" {
  for (b, sep, found, before, after) in CUT_CASES {
    let (f, x, y) = match b.cut(sep) {
      Some((p, q)) => (true, p, q)
      None => (false, b"", b"")
    }
    expect((b, sep, f, x, y)) == (b, sep, found, before, after)
  }
}

test "cut_prefix is Go's CutPrefix" {
  for (b, p, found, after) in CUTP_CASES {
    let (f, x) = match b.cut_prefix(p) {
      Some(y) => (true, y)
      None => (false, b"")
    }
    expect((b, p, f, x)) == (b, p, found, after)
  }
}

test "cut_suffix is Go's CutSuffix" {
  for (b, p, found, before) in CUTS_CASES {
    let (f, x) = match b.cut_suffix(p) {
      Some(y) => (true, y)
      None => (false, b"")
    }
    expect((b, p, f, x)) == (b, p, found, before)
  }
}

test "split_after and split_after_n are Go's" {
  for (b, sep, n, want) in SPLITAFTER_CASES {
    expect((b, sep, n, b.split_after_n(sep, n))) == (b, sep, n, want)
    if n < 0 {
      expect((b, sep, b.split_after(sep))) == (b, sep, want)
    }
  }
}

test "the search and cut family on a view of a longer value" {
  // "zz" + b sliced back is a view of a longer Bytes, so every piece and
  // every found offset is a view's
  for (b, chars, want) in ANY_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    expect((b, chars, any_of(view, chars))) == (b, chars, want)
  }
  for (b, sep, found, before, after) in CUT_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    let (f, x, y) = match view.cut(sep) {
      Some((p, q)) => (true, p, q)
      None => (false, b"", b"")
    }
    expect((b, sep, f, x, y)) == (b, sep, found, before, after)
  }
  for (b, p, found, after) in CUTP_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    let (f, x) = match view.cut_prefix(p) {
      Some(y) => (true, y)
      None => (false, b"")
    }
    expect((b, p, f, x)) == (b, p, found, after)
  }
  for (b, p, found, before) in CUTS_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    let (f, x) = match view.cut_suffix(p) {
      Some(y) => (true, y)
      None => (false, b"")
    }
    expect((b, p, f, x)) == (b, p, found, before)
  }
  for (b, sep, n, want) in SPLITAFTER_CASES {
    let long = b"zz" + b
    let view = long[2..long.len()]
    expect((b, sep, n, view.split_after_n(sep, n))) == (b, sep, n, want)
  }
}
`
