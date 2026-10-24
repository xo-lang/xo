package main

import (
	"fmt"
	"math/rand"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The predicate and mapping tags the corpus uses: one Xo function per tag
// (pred and mapf in the generated test), and the Go function beside the
// tag here, so the oracle and the test run the same predicate.

// wherePreds are the predicates of find_where, rfind_where, contains_where,
// trim_where*, split_where and their names.
var wherePreds = []struct {
	name string
	in   func(rune) bool
}{
	{"is_space", unicode.IsSpace},
	{"is_digit", unicode.IsDigit},
	{"is_upper", unicode.IsUpper},
	{"not_fffd", func(r rune) bool { return r != utf8.RuneError }},
	{"not_space", func(r rune) bool { return !unicode.IsSpace(r) }},
	{"not_digit", func(r rune) bool { return !unicode.IsDigit(r) }},
	{"is_x", func(r rune) bool { return r == 'X' }},
	{"always", func(rune) bool { return true }},
	{"never", func(rune) bool { return false }},
	{"is_lower_a_z", func(r rune) bool { return r >= 'a' && r <= 'z' }},
}

// whereMaps are the mappings of map_runes: -1 is None (Go's negative
// mapping, a dropped rune).
var whereMaps = []struct {
	name string
	f    func(rune) int
}{
	{"identity", func(r rune) int { return int(r) }},
	{"rot13", whereRot13},
	{"keep_ascii_letters", whereKeepASCII},
	{"to_upper", func(r rune) int { return int(unicode.ToUpper(r)) }},
	{"max_rune", func(rune) int { return unicode.MaxRune }},
	{"letter_a", func(rune) int { return 'a' }},
	{"ascii_or_fffd", whereASCIIOrFFFD},
	{"drop_space", whereDropSpace},
	{"swap_rune_self", whereSwapRuneSelf},
	{"to_lower", func(r rune) int { return int(unicode.ToLower(r)) }},
}

// whereRot13 is Go's strings_test.go rot13.
func whereRot13(r rune) int {
	switch {
	case r >= 'a' && r <= 'z':
		return int('a' + (r-'a'+13)%26)
	case r >= 'A' && r <= 'Z':
		return int('A' + (r-'A'+13)%26)
	}
	return int(r)
}

// whereKeepASCII keeps ASCII letters and drops the rest (Go's
// dropNotLatin on the corpus inputs, which are Latin only in that sense).
func whereKeepASCII(r rune) int {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		return int(r)
	}
	return -1
}

// whereASCIIOrFFFD keeps ASCII letters, replaces the rest with U+FFFD (Go's
// replaceNotLatin on the corpus inputs).
func whereASCIIOrFFFD(r rune) int {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		return int(r)
	}
	return int(utf8.RuneError)
}

// whereDropSpace is Go's trimSpaces of TestMap: spaces dropped.
func whereDropSpace(r rune) int {
	if unicode.IsSpace(r) {
		return -1
	}
	return int(r)
}

// whereSwapRuneSelf swaps U+0080 and the last code point (Go's encode of
// TestMap).
func whereSwapRuneSelf(r rune) int {
	switch r {
	case utf8.RuneSelf:
		return unicode.MaxRune
	case unicode.MaxRune:
		return utf8.RuneSelf
	}
	return int(r)
}

// whereApply runs mapping tag m on r the way Xo's map_runes will.
func whereApply(m int, r rune) (rune, bool) {
	v := whereMaps[m].f(r)
	if v < 0 {
		return 0, false
	}
	return rune(v), true
}

// whereMapString is Go's strings.Map of tag m over s, in the form the
// corpus stores.
func whereMapString(m int, s string) string {
	var b strings.Builder
	for _, r := range s {
		if v, ok := whereApply(m, r); ok {
			b.WriteRune(v)
		}
	}
	return b.String()
}

// whereListOf is a List[Str] the way the corpora encode it: its length,
// U+001F, and the pieces joined by U+0000.
func whereListOf(parts []string) string {
	return fmt.Sprintf("%d\x1f%s", len(parts), strings.Join(parts, "\x00"))
}

// whereSplit is strings.FieldsFunc with predicate tag p, encoded.
func whereSplit(p int, s string) string {
	return whereListOf(strings.FieldsFunc(s, wherePreds[p].in))
}

// whereTrim is the three trims of Go's trimFuncTests with tag p.
func whereTrim(p int, s string) (string, string, string) {
	return strings.TrimFunc(s, wherePreds[p].in), strings.TrimLeftFunc(s, wherePreds[p].in), strings.TrimRightFunc(s, wherePreds[p].in)
}

// whereIndex is Go's IndexFunc, LastIndexFunc and ContainsFunc with tag p,
// as the corpus joins them.
func whereIndex(p int, s string) string {
	return fmt.Sprintf("%d\x1f%d\x1f%t", strings.IndexFunc(s, wherePreds[p].in), strings.LastIndexFunc(s, wherePreds[p].in), strings.ContainsFunc(s, wherePreds[p].in))
}

// whereCase is one (input, tag) of a corpus family.
type whereCase struct {
	s   string
	tag int
}

// whereSpace is Go's strings_test.go space.
const whereSpace = "\t\v\r\f\n\u0085  　"

// whereThai is the Thai digit string of Go's indexFuncTests and
// trimFuncTests: U+0E50 U+0E52 "12hello34" U+0E50 U+0E51.
const whereThai = "๐๒12hello34๐๑"

// whereEarth is the Ɐ string of Go's tables.
const whereEarth = "ⱯⱯⱯⱯABCDhelloEFⱯⱯGHⱯⱯ"

// goWhereIndexCases are Go's strings_test.go indexFuncTests.
// Go's strings tests are Copyright 2009 The Go Authors, BSD-style
// license.
func goWhereIndexCases() []whereCase {
	return []whereCase{
		{"", 3}, {"abc", 1}, {"0123", 1}, {"a1b", 1},
		{whereSpace, 0}, {whereThai, 1}, {whereEarth, 2},
		{"12๐๒hello34๐๑", 5},
		// Go's invalid UTF-8 rows, in the U+FFFD an Xo Str sees
		// (decision 0259): the bytes were written \x80, \xc0, \xe0.
		{"�1", 1}, {"�abc", 1}, {"�a�", 3},
		{"�a�", 6}, {"�☺�", 6},
		{"�☺��", 6}, {"ab�a�cd", 6},
		{"a��cd", 6}, {"����", 6},
	}
}

// goWhereTrimCases are Go's strings_test.go trimFuncTests with the same
// conversion.
func goWhereTrimCases() []whereCase {
	return []whereCase{
		{whereSpace + " hello " + whereSpace, 0},
		{whereThai, 1},
		{whereEarth, 2},
		{"hello" + whereSpace + "hello", 4},
		{"hello๐๒1234๐๑helo", 5},
		{"ab�a�cd", 3},
		{"�a�", 6},
		{"", 0}, {" ", 0},
	}
}

// goWhereSplitCases are Go's FieldsFuncTests with fieldstests (the
// unicode.IsSpace predicate) and the 'X' predicate of TestFieldsFunc.
func goWhereSplitCases() []whereCase {
	cs := []whereCase{
		{"", 6}, {"XX", 6}, {"XXhiXXX", 6}, {"aXXbXXXcX", 6},
	}
	for _, s := range []string{
		"", " ", "  ", "   ", "\t", "\n", "\t\n\x0b\x0c\r ", "x", " x ",
		"1 2 3", "1\t2\t3", "1\t\t2\t\t3\t4", "1 2 3 4",
		"   ", "\n™\t™\n", "\n 1™2    ™",
		"\n1� �2 3�4", "1� �2� �",
		"☺☺☺", "\U0001F600\U0001F600 \U0001F600",
	} {
		cs = append(cs, whereCase{s, 0})
	}
	return cs
}

// goWhereMapCases are the cases of Go's TestMap (TestMap's grow, shrink,
// rot13, rot13 twice, drop, identity, a malformed byte, the RuneSelf and
// MaxRune encoding, and spaces dropped).
func goWhereMapCases() []whereCase {
	return []whereCase{
		{whereTenRunes('a'), 4},             // grow
		{whereTenRunes(unicode.MaxRune), 5}, // shrink
		{"a to zed", 1},                     // rot13
		{"n gb mrq", 1},                     // rot13 again
		{"Hello, 세계", 2},          // drop
		{"Input string that we expect not to be copied.", 0}, // identity
		{"Hello�World", 6}, // a malformed byte, as U+FFFD
		{string(rune(utf8.RuneSelf)) + string(unicode.MaxRune), 8},
		{string(unicode.MaxRune) + string(rune(utf8.RuneSelf)), 8},
		{"  a  b  c  ", 7},
		{"hello", 3}, {"HELLO", 9},
	}
}

// whereTenRunes is Go's tenRunes of TestMap.
func whereTenRunes(r rune) string {
	b := make([]rune, 10)
	for i := range b {
		b[i] = r
	}
	return string(b)
}

// whereAlphabet is the alphabet of the random inputs: ASCII, runes of
// every UTF-8 size, U+FFFD, broken bytes (which an Xo Str sees as U+FFFD:
// decision 0259) and the space runes.
var whereAlphabet = []string{
	"a", "b", "z", "A", "Z", "0", "7", "X", " ", "\t", "\n",
	" ", " ", "　", ".", "-", "_", "é", "ß",
	"σ", "๐", "Ɐ", "™", "☺", "�",
	"世", "\U0001F600", "\U0010FFFF", "\x80", "\xc0", "\xe0", "\xff",
}

func whereGen(r *rand.Rand, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(whereAlphabet[r.Intn(len(whereAlphabet))])
	}
	return textOf(b.String())
}

var whereLens = []int{0, 1, 2, 3, 4, 5, 6, 8, 11, 16, 23, 40}

func whereRandom(r *rand.Rand, tags int) whereCase {
	return whereCase{whereGen(r, whereLens[r.Intn(len(whereLens))]), r.Intn(tags)}
}

func whereCorpus() string {
	// each family has its own seed, so adding one leaves the others alone
	r := rand.New(rand.NewSource(43))

	var o strings.Builder
	o.WriteString(header("Go's strings.IndexFunc, LastIndexFunc, ContainsFunc, TrimFunc, TrimLeftFunc, TrimRightFunc, FieldsFunc and Map on the inputs of Go's strings_test.go (indexFuncTests, trimFuncTests, FieldsFuncTests with fieldstests, TestMap) and on seeded random inputs, against Str.find_where, rfind_where, contains_where, trim_where, trim_start_where, trim_end_where, split_where and map_runes (Go's strings tests are Copyright 2009 The Go Authors, BSD-style license). The predicates and mappings are tags: pred and mapf in the test below hold the Xo form of each, the same function the Go oracle ran. Go's invalid UTF-8 inputs are written as the U+FFFD an Xo Str sees (decision 0259)"))

	idx := goWhereIndexCases()
	for i := 0; i < 2000; i++ {
		idx = append(idx, whereRandom(r, len(wherePreds)))
	}
	o.WriteString("\n/// (s, predicate tag, Go's IndexFunc, LastIndexFunc and ContainsFunc\n/// joined by U+001F, None as -1)\nconst INDEX_CASES: List[(Str, Int, Str)] = [\n")
	for _, c := range idx {
		c.s = textOf(c.s)
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xq(c.s), c.tag, xq(whereIndex(c.tag, c.s)))
	}
	o.WriteString("]\n")

	tr := goWhereTrimCases()
	rt := rand.New(rand.NewSource(47))
	for i := 0; i < 2000; i++ {
		tr = append(tr, whereRandom(rt, len(wherePreds)))
	}
	o.WriteString("\n/// (s, predicate tag, Go's TrimFunc, TrimLeftFunc and TrimRightFunc)\nconst TRIM_CASES: List[(Str, Int, Str, Str, Str)] = [\n")
	for _, c := range tr {
		c.s = textOf(c.s)
		t, l, rr := whereTrim(c.tag, c.s)
		fmt.Fprintf(&o, "  (%s, %d, %s, %s, %s),\n", xq(c.s), c.tag, xq(t), xq(l), xq(rr))
	}
	o.WriteString("]\n")

	sp := goWhereSplitCases()
	rs := rand.New(rand.NewSource(53))
	for i := 0; i < 2000; i++ {
		sp = append(sp, whereRandom(rs, len(wherePreds)))
	}
	o.WriteString("\n/// (s, predicate tag, Go's FieldsFunc as its length and its pieces\n/// joined by U+0000, joined by U+001F)\nconst SPLIT_CASES: List[(Str, Int, Str)] = [\n")
	for _, c := range sp {
		c.s = textOf(c.s)
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xq(c.s), c.tag, xq(whereSplit(c.tag, c.s)))
	}
	o.WriteString("]\n")

	mp := goWhereMapCases()
	rm := rand.New(rand.NewSource(59))
	for i := 0; i < 2000; i++ {
		mp = append(mp, whereRandom(rm, len(whereMaps)))
	}
	o.WriteString("\n/// (s, mapping tag, Go's Map)\nconst MAP_CASES: List[(Str, Int, Str)] = [\n")
	for _, c := range mp {
		c.s = textOf(c.s)
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xq(c.s), c.tag, xq(whereMapString(c.tag, c.s)))
	}
	o.WriteString("]\n")

	o.WriteString(whereTests)
	return o.String()
}

const whereTests = `
// The predicates and mappings the tags name, the same ones the Go oracle
// ran (scripts/stdcorpus/where.go).

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

fn list_of(xs: List[Str]) -> Str {
  "${xs.len()}\u{1f}${xs.join("\u{0}")}"
}

fn show4(bad: List[(Str, Int, Str, Str)]) -> List[(Str, Int, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "find_where, rfind_where and contains_where are Go's *Func" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (s, tag, want) in INDEX_CASES {
    let got = "${s.find_where(fn(r) { pred(tag, r) }) ?? -1}\u{1f}${s.rfind_where(fn(r) { pred(tag, r) }) ?? -1}\u{1f}${s.contains_where(fn(r) { pred(tag, r) })}"
    if got != want {
      bad.push((s, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}

test "trim_where, trim_start_where and trim_end_where are Go's *Func" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (s, tag, wt, wl, wr) in TRIM_CASES {
    let t = s.trim_where(fn(r) { pred(tag, r) })
    let l = s.trim_start_where(fn(r) { pred(tag, r) })
    let r = s.trim_end_where(fn(r) { pred(tag, r) })
    if t != wt || l != wl || r != wr {
      bad.push((s, tag, "${t}\u{1f}${l}\u{1f}${r}", "${wt}\u{1f}${wl}\u{1f}${wr}"))
    }
  }
  expect(show4(bad)) == []
}

test "split_where is Go's FieldsFunc" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (s, tag, want) in SPLIT_CASES {
    let got = list_of(s.split_where(fn(r) { pred(tag, r) }))
    if got != want {
      bad.push((s, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}

test "map_runes is Go's Map" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (s, tag, want) in MAP_CASES {
    let got = s.map_runes(fn(r) { mapf(tag, r) })
    if got != want {
      bad.push((s, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}

// The same inputs as a view of a longer buffer: the pieces and trims of a
// view keep their source alive and do not see the buffer's other bytes.

test "the trims and split_where of a view" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (s, tag, wt, wl, wr) in TRIM_CASES {
    let long = s + "Q"
    let view = long[0..s.len()]
    let t = view.trim_where(fn(r) { pred(tag, r) })
    let l = view.trim_start_where(fn(r) { pred(tag, r) })
    let r = view.trim_end_where(fn(r) { pred(tag, r) })
    if t != wt || l != wl || r != wr {
      bad.push((s, tag, "${t}\u{1f}${l}\u{1f}${r}", "${wt}\u{1f}${wl}\u{1f}${wr}"))
    }
  }
  for (s, tag, want) in SPLIT_CASES {
    let long = s + "Q"
    let view = long[0..s.len()]
    let got = list_of(view.split_where(fn(r) { pred(tag, r) }))
    if got != want {
      bad.push((s, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}

test "map_runes of a view" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (s, tag, want) in MAP_CASES {
    let long = s + "Q"
    let view = long[0..s.len()]
    let got = view.map_runes(fn(r) { mapf(tag, r) })
    if got != want {
      bad.push((s, tag, got, want))
    }
  }
  expect(show4(bad)) == []
}
`
