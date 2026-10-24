package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The Bytes predicate corpus: Go's bytes.IndexFunc, LastIndexFunc,
// ContainsFunc, TrimFunc, TrimLeftFunc, TrimRightFunc, FieldsFunc and Map
// against Bytes.find_where, rfind_where, contains_where, trim_where,
// trim_start_where, trim_end_where, split_where and map_runes.
//
// The predicates and mappings are the tags of where.go (wherePreds and
// whereMaps), so the same Go function the oracle ran is the one the Xo test
// runs. Unlike the Str corpus, the inputs keep their invalid UTF-8: a Bytes
// holds raw bytes, and both Go and Xo decode an invalid byte as U+FFFD, so
// nothing has to be rewritten (decision 0259 is about Str).

// whereGenRaw is whereGen without the textOf rewrite: the bytes as written,
// invalid sequences included.
func whereGenRaw(r *rand.Rand, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(whereAlphabet[r.Intn(len(whereAlphabet))])
	}
	return b.String()
}

func bytesWhereRandom(r *rand.Rand, tags int) whereCase {
	return whereCase{whereGenRaw(r, whereLens[r.Intn(len(whereLens))]), r.Intn(tags)}
}

// bytesListOf is a List[Bytes] the way the bytes corpora encode it: its
// length, U+001F, and the pieces in hex joined by spaces.
func bytesListOf(parts [][]byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d", len(parts))
	for _, p := range parts {
		b.WriteString(" ")
		b.WriteString(xhex(p))
	}
	return b.String()
}

// xhex is a Bytes in the corpus's hex form: the Xo test prints to_hex().
func xhex(b []byte) string {
	const digits = "0123456789abcdef"
	var o strings.Builder
	for _, c := range b {
		o.WriteByte(digits[c>>4])
		o.WriteByte(digits[c&0xf])
	}
	return o.String()
}

// bytesWhereIndex is Go's IndexFunc, LastIndexFunc and ContainsFunc with the
// predicate tag, joined as the Str corpus joins them.
func bytesWhereIndex(p int, b []byte) string {
	f := func(r rune) bool { return wherePreds[p].in(r) }
	return fmt.Sprintf("%d\x1f%d\x1f%t", bytes.IndexFunc(b, f), bytes.LastIndexFunc(b, f), bytes.ContainsFunc(b, f))
}

// bytesWhereTrim is the three trims of Go's trimFuncTests with a tag.
func bytesWhereTrim(p int, b []byte) ([]byte, []byte, []byte) {
	f := func(r rune) bool { return wherePreds[p].in(r) }
	return bytes.TrimFunc(b, f), bytes.TrimLeftFunc(b, f), bytes.TrimRightFunc(b, f)
}

// bytesWhereMap is Go's bytes.Map of mapping tag m.
func bytesWhereMap(m int, b []byte) []byte {
	return bytes.Map(func(r rune) rune { return rune(whereMaps[m].f(r)) }, b)
}

// goBytesWhereIndexCases are the inputs of Go's bytes tests for the family
// (the strings_test.go tables, over raw bytes with the invalid sequences
// kept) plus the bytes package's own edge rows.
// Go's tests are Copyright 2009 The Go Authors, BSD-style license.
func goBytesWhereIndexCases() []whereCase {
	return []whereCase{
		{"", 3}, {"abc", 1}, {"0123", 1}, {"a1b", 1},
		{whereSpace, 0}, {whereThai, 1}, {whereEarth, 2},
		{"12๐๒hello34๐๑", 5},
		{"\x801", 1}, {"\x80abc", 1}, {"\x80a\x80", 3},
		{"\x80a\x80", 6}, {"\x80☺\x80", 6},
		{"\x80☺\x80\x80", 6}, {"ab\x80a\x80cd", 6},
		{"a\x80\x80cd", 6}, {"\x80\x80\x80\x80", 6},
		// a two byte sequence split across the value: the tail byte alone
		// is invalid and reads as U+FFFD
		{"\xc0\x80", 3}, {"a\xe0\x80z", 3},
	}
}

// goBytesWhereTrimCases are the trim inputs: strings_test.go's trimFuncTests
// over raw bytes, and the bytes package's invalid caught in the middle.
func goBytesWhereTrimCases() []whereCase {
	return []whereCase{
		{whereSpace + " hello " + whereSpace, 0},
		{whereThai, 1},
		{whereEarth, 2},
		{"hello" + whereSpace + "hello", 4},
		{"hello๐๒1234๐๑helo", 5},
		{"ab\x80a\x80cd", 3},
		{"\x80a\x80", 6},
		{"", 0}, {" ", 0},
	}
}

// goBytesWhereSplitCases are Go's FieldsFunc inputs: fieldstests with
// unicode.IsSpace and TestFieldsFunc's 'X' rows.
func goBytesWhereSplitCases() []whereCase {
	cs := []whereCase{
		{"", 6}, {"XX", 6}, {"XXhiXXX", 6}, {"aXXbXXXcX", 6},
	}
	for _, s := range []string{
		"", " ", "  ", "   ", "\t", "\n", "\t\n\x0b\x0c\r ", "x", " x ",
		"1 2 3", "1\t2\t3", "1\t\t2\t\t3\t4", "1 2 3 4",
		"   ", "\n™\t™\n", "\n 1™2   ™",
		"\n1\x80 \x802 3\x804", "1\x80 \x802\x80  ",
		"☺☺☺", "\U0001F600\U0001F600 \U0001F600",
	} {
		cs = append(cs, whereCase{s, 0})
	}
	return cs
}

// goBytesWhereMapCases are the bytes package's Map inputs plus TestMap's.
func goBytesWhereMapCases() []whereCase {
	return []whereCase{
		{whereTenRunes('a'), 4},                              // grow
		{whereTenRunes(unicode.MaxRune), 5},                  // shrink
		{"a to zed", 1},                                      // rot13
		{"n gb mrq", 1},                                      // rot13 again
		{"Hello, 세계", 2},                           // drop
		{"Input string that we expect not to be copied.", 0}, // identity
		{"Hello\x80World", 6},                                // a malformed byte, kept as U+FFFD
		{string(rune(utf8.RuneSelf)) + string(unicode.MaxRune), 8},
		{string(unicode.MaxRune) + string(rune(utf8.RuneSelf)), 8},
		{"  a  b  c  ", 7},
		{"hello", 3}, {"HELLO", 9},
		{"\x80\x80", 0}, {"\x80\x80", 7},
	}
}

// byteswhereCorpus appends the Bytes predicate sections to the bytes corpus.
func byteswhereCorpus() string {
	r := rand.New(rand.NewSource(61))

	var o strings.Builder
	o.WriteString(header("Go's bytes.IndexFunc, LastIndexFunc, ContainsFunc, TrimFunc, TrimLeftFunc, TrimRightFunc, FieldsFunc and Map on the inputs of Go's strings_test.go and bytes tests, with invalid UTF-8 kept as raw bytes, and on seeded random inputs, against Bytes.find_where, rfind_where, contains_where, trim_where, trim_start_where, trim_end_where, split_where and map_runes (Go's tests are Copyright 2009 The Go Authors, BSD-style license). The predicates and mappings are tags: pred and mapf in the test below hold the Xo form of each, the same function the Go oracle ran"))

	idx := goBytesWhereIndexCases()
	for i := 0; i < 2000; i++ {
		idx = append(idx, bytesWhereRandom(r, len(wherePreds)))
	}
	o.WriteString("\n/// (b, predicate tag, Go's IndexFunc, LastIndexFunc and ContainsFunc\n/// joined by U+001F, None as -1)\nconst WHERE_INDEX_CASES: List[(Bytes, Int, Str)] = [\n")
	for _, c := range idx {
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xbytes([]byte(c.s)), c.tag, xq(bytesWhereIndex(c.tag, []byte(c.s))))
	}
	o.WriteString("]\n")
	tr := goBytesWhereTrimCases()
	rt := rand.New(rand.NewSource(67))
	for i := 0; i < 2000; i++ {
		tr = append(tr, bytesWhereRandom(rt, len(wherePreds)))
	}
	o.WriteString("\n/// (b, predicate tag, Go's TrimFunc, TrimLeftFunc and TrimRightFunc)\nconst WHERE_TRIM_CASES: List[(Bytes, Int, Bytes, Bytes, Bytes)] = [\n")
	for _, c := range tr {
		t, l, rr := bytesWhereTrim(c.tag, []byte(c.s))
		fmt.Fprintf(&o, "  (%s, %d, %s, %s, %s),\n", xbytes([]byte(c.s)), c.tag, xbytes(t), xbytes(l), xbytes(rr))
	}
	o.WriteString("]\n")

	sp := goBytesWhereSplitCases()
	rs := rand.New(rand.NewSource(71))
	for i := 0; i < 2000; i++ {
		sp = append(sp, bytesWhereRandom(rs, len(wherePreds)))
	}
	o.WriteString("\n/// (b, predicate tag, Go's FieldsFunc as its length, then its pieces in\n/// hex joined by spaces)\nconst WHERE_SPLIT_CASES: List[(Bytes, Int, Str)] = [\n")
	for _, c := range sp {
		f := func(rn rune) bool { return wherePreds[c.tag].in(rn) }
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xbytes([]byte(c.s)), c.tag, xq(bytesListOf(bytes.FieldsFunc([]byte(c.s), f))))
	}
	o.WriteString("]\n")

	mp := goBytesWhereMapCases()
	rm := rand.New(rand.NewSource(73))
	for i := 0; i < 2000; i++ {
		mp = append(mp, bytesWhereRandom(rm, len(whereMaps)))
	}
	o.WriteString("\n/// (b, mapping tag, Go's Map)\nconst WHERE_MAP_CASES: List[(Bytes, Int, Bytes)] = [\n")
	for _, c := range mp {
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xbytes([]byte(c.s)), c.tag, xbytes(bytesWhereMap(c.tag, []byte(c.s))))
	}
	o.WriteString("]\n")

	o.WriteString(byteswhereTests)
	return o.String()
}

const byteswhereTests = `
// The predicates and mappings the tags name, the same ones the Go oracle
// ran (scripts/stdcorpus/where.go and byteswhere.go).

fn pred(tag: Int, r: Rune) -> Bool {
  if tag == 0 {
    r.is_space()
  } else if tag == 1 {
    r.is_digit()
  } else if tag == 2 {
    r.is_upper()
  } else if tag == 3 {
    r != Rune(0xfffd)
  } else if tag == 4 {
    !r.is_space()
  } else if tag == 5 {
    !r.is_digit()
  } else if tag == 6 {
    r == Rune(88)
  } else if tag == 7 {
    true
  } else if tag == 8 {
    false
  } else {
    r >= Rune(97) && r <= Rune(122)
  }
}

fn rot13(r: Rune) -> Rune {
  if r >= Rune(97) && r <= Rune(122) {
    Rune(97 + (Int(r) - 97 + 13) % 26)
  } else if r >= Rune(65) && r <= Rune(90) {
    Rune(65 + (Int(r) - 65 + 13) % 26)
  } else {
    r
  }
}

fn mapf(tag: Int, r: Rune) -> Rune? {
  let letter = (r >= Rune(97) && r <= Rune(122)) || (r >= Rune(65) && r <= Rune(90))
  if tag == 0 {
    Some(r)
  } else if tag == 1 {
    Some(rot13(r))
  } else if tag == 2 {
    if letter { Some(r) } else { None }
  } else if tag == 3 {
    Some(r.to_upper())
  } else if tag == 4 {
    Some(Rune(0x10ffff))
  } else if tag == 5 {
    Some(Rune(97))
  } else if tag == 6 {
    if letter { Some(r) } else { Some(Rune(0xfffd)) }
  } else if tag == 7 {
    if r.is_space() { None } else { Some(r) }
  } else if tag == 8 {
    if Int(r) == 0x80 {
      Some(Rune(0x10ffff))
    } else if Int(r) == 0x10ffff {
      Some(Rune(0x80))
    } else {
      Some(r)
    }
  } else {
    Some(r.to_lower())
  }
}

fn where_list_of(xs: List[Bytes]) -> Str {
  var out = "${xs.len()}"
  for x in xs {
    out = "${out} ${x.to_hex()}"
  }
  out
}

fn show4(bad: List[(Bytes, Int, Str, Str)]) -> List[(Bytes, Int, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "find_where, rfind_where and contains_where are Go's *Func" {
  var bad: List[(Bytes, Int, Str, Str)] = []
  for (b, tag, want) in WHERE_INDEX_CASES {
    let got = "${b.find_where(fn(r) { pred(tag, r) }) ?? -1}\u{1f}${b.rfind_where(fn(r) { pred(tag, r) }) ?? -1}\u{1f}${b.contains_where(fn(r) { pred(tag, r) })}"
    if got != want {
      bad.push((b, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}

test "trim_where, trim_start_where and trim_end_where are Go's *Func" {
  var bad: List[(Bytes, Int, Str, Str)] = []
  for (b, tag, wt, wl, wr) in WHERE_TRIM_CASES {
    let t = b.trim_where(fn(r) { pred(tag, r) })
    let l = b.trim_start_where(fn(r) { pred(tag, r) })
    let r = b.trim_end_where(fn(r) { pred(tag, r) })
    if t != wt || l != wl || r != wr {
      bad.push((b, tag, "${t.to_hex()}\u{1f}${l.to_hex()}\u{1f}${r.to_hex()}", "${wt.to_hex()}\u{1f}${wl.to_hex()}\u{1f}${wr.to_hex()}"))
    }
  }
  expect(show4(bad)) == []
}

test "split_where is Go's FieldsFunc" {
  var bad: List[(Bytes, Int, Str, Str)] = []
  for (b, tag, want) in WHERE_SPLIT_CASES {
    let xs = b.split_where(fn(r) { pred(tag, r) })
    let got = where_list_of(xs)
    if got != want {
      bad.push((b, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}

test "map_runes is Go's Map" {
  var bad: List[(Bytes, Int, Str, Str)] = []
  for (b, tag, want) in WHERE_MAP_CASES {
    let got = b.map_runes(fn(r) { mapf(tag, r) })
    if got != want {
      bad.push((b, tag, got.to_hex(), want.to_hex()))
    }
  }
  expect(show4(bad)) == []
}

// The same inputs as a view of a longer buffer: a Bytes view shares its
// source on native targets, and the pieces and trims see only the view.

test "the trims and split_where of a view" {
  var bad: List[(Bytes, Int, Str, Str)] = []
  for (b, tag, wt, wl, wr) in WHERE_TRIM_CASES {
    let long = b + b"Q"
    let view = long[0..b.len()]
    let t = view.trim_where(fn(r) { pred(tag, r) })
    let l = view.trim_start_where(fn(r) { pred(tag, r) })
    let r = view.trim_end_where(fn(r) { pred(tag, r) })
    if t != wt || l != wl || r != wr {
      bad.push((b, tag, "${t.to_hex()}\u{1f}${l.to_hex()}\u{1f}${r.to_hex()}", "${wt.to_hex()}\u{1f}${wl.to_hex()}\u{1f}${wr.to_hex()}"))
    }
  }
  for (b, tag, want) in WHERE_SPLIT_CASES {
    let long = b + b"Q"
    let view = long[0..b.len()]
    let xs = view.split_where(fn(r) { pred(tag, r) })
    let got = where_list_of(xs)
    if got != want {
      bad.push((b, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}

test "map_runes of a view" {
  var bad: List[(Bytes, Int, Str, Str)] = []
  for (b, tag, want) in WHERE_MAP_CASES {
    let long = b + b"Q"
    let view = long[0..b.len()]
    let got = view.map_runes(fn(r) { mapf(tag, r) })
    if got != want {
      bad.push((b, tag, got.to_hex(), want.to_hex()))
    }
  }
  expect(show4(bad)) == []
}
`
