package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// strsearchCase is one (s, sep) input of the Str search and split methods.
type strsearchCase struct{ s, sep string }

// The API these corpora cover, as std/prelude.xo declares it: Str.find
// (strings.Index), Str.rfind (strings.LastIndex), Str.contains
// (strings.Contains), Str.split (strings.Split), Str.replace
// (strings.ReplaceAll) and List[Str].join (strings.Join). Go's
// strings.IndexAny, LastIndexAny, Count, Fields and the n of Replace have
// no Str method, so their tables are not here; replace_all is
// Str.replace, which always replaces every match.

// goIndexCases are Go's strings_test.go indexTests and lastIndexTests
// (IndexTest) merged: each case checks Index, LastIndex and Contains.
// Go's strings tests are Copyright 2009 The Go Authors, BSD-style
// license.
func goIndexCases() []strsearchCase {
	var cs []strsearchCase
	for _, t := range [][2]string{
		{"", ""}, {"", "a"}, {"", "foo"}, {"fo", "foo"}, {"foo", "foo"},
		{"oofofoofooo", "f"}, {"oofofoofooo", "foo"}, {"barfoobarfoo", "foo"},
		{"foo", ""}, {"foo", "o"}, {"abcABCabc", "A"},
		{"jrzm6jjhorimglljrea4w3rlgosts0w2gia17hno2td4qd1jz", "jz"},
		{"ekkuk5oft4eq0ocpacknhwouic1uua46unx12l37nioq9wbpnocqks6", "ks6"},
		{"999f2xmimunbuyew5vrkla9cpwhmxan8o98ec", "98ec"},
		{"9lpt9r98i04k8bz6c6dsrthb96bhi", "96bhi"},
		{"55u558eqfaod2r2gu42xxsu631xf0zobs5840vl", "5840vl"},
		{"x", "a"}, {"x", "x"}, {"abc", "a"}, {"abc", "b"}, {"abc", "c"}, {"abc", "x"},
		{"", "ab"}, {"bc", "ab"}, {"ab", "ab"}, {"xab", "ab"}, {"xab"[:2], "ab"},
		{"", "abc"}, {"xbc", "abc"}, {"abc", "abc"}, {"xabc", "abc"}, {"xabc"[:3], "abc"}, {"xabxc", "abc"},
		{"", "abcd"}, {"xbcd", "abcd"}, {"abcd", "abcd"}, {"xabcd", "abcd"}, {"xyabcd"[:5], "abcd"},
		{"xbcqq", "abcqq"}, {"abcqq", "abcqq"}, {"xabcqq", "abcqq"}, {"xyabcqq"[:6], "abcqq"},
		{"xabxcqq", "abcqq"}, {"xabcqxq", "abcqq"},
		{"", "01234567"}, {"32145678", "01234567"}, {"01234567", "01234567"},
		{"x01234567", "01234567"}, {"x0123456x01234567", "01234567"}, {"xx01234567"[:9], "01234567"},
		{"", "0123456789"}, {"3214567844", "0123456789"}, {"0123456789", "0123456789"},
		{"x0123456789", "0123456789"}, {"x012345678x0123456789", "0123456789"},
		{"xyz0123456789"[:12], "0123456789"}, {"x01234567x89", "0123456789"},
		{"", "0123456789012345"}, {"3214567889012345", "0123456789012345"},
		{"0123456789012345", "0123456789012345"}, {"x0123456789012345", "0123456789012345"},
		{"x012345678901234x0123456789012345", "0123456789012345"},
		{"", "01234567890123456789"}, {"32145678890123456789", "01234567890123456789"},
		{"01234567890123456789", "01234567890123456789"},
		{"x01234567890123456789", "01234567890123456789"},
		{"x0123456789012345678x01234567890123456789", "01234567890123456789"},
		{"xyz01234567890123456789"[:22], "01234567890123456789"},
		{"", "0123456789012345678901234567890"},
		{"321456788901234567890123456789012345678911", "0123456789012345678901234567890"},
		{"0123456789012345678901234567890", "0123456789012345678901234567890"},
		{"x0123456789012345678901234567890", "0123456789012345678901234567890"},
		{"x012345678901234567890123456789x0123456789012345678901234567890", "0123456789012345678901234567890"},
		{"xyz0123456789012345678901234567890"[:33], "0123456789012345678901234567890"},
		{"", "01234567890123456789012345678901"},
		{"32145678890123456789012345678901234567890211", "01234567890123456789012345678901"},
		{"01234567890123456789012345678901", "01234567890123456789012345678901"},
		{"x01234567890123456789012345678901", "01234567890123456789012345678901"},
		{"x0123456789012345678901234567890x01234567890123456789012345678901", "01234567890123456789012345678901"},
		{"xyz01234567890123456789012345678901"[:34], "01234567890123456789012345678901"},
		{"xxxxxx012345678901234567890123456789012345678901234567890123456789012", "012345678901234567890123456789012345678901234567890123456789012"},
		{"", "0123456789012345678901234567890123456789"},
		{"xx012345678901234567890123456789012345678901234567890123456789012", "0123456789012345678901234567890123456789"},
		{"xx012345678901234567890123456789012345678901234567890123456789012"[:41], "0123456789012345678901234567890123456789"},
		{"xx012345678901234567890123456789012345678901234567890123456789012", "0123456789012345678901234567890123456xxx"},
		{"xx0123456789012345678901234567890123456789012345678901234567890120123456789012345678901234567890123456xxx", "0123456789012345678901234567890123456xxx"},
		{"oxoxoxoxoxoxoxoxoxoxoxoy", "oy"}, {"oxoxoxoxoxoxoxoxoxoxoxox", "oy"},
		{"oxoxoxoxoxoxoxoxoxoxox☺", "☺"},
		{"xx0123456789012345678901234567890123456789012345678901234567890120123456789012345678901234567890123456xxx\xed\x9f\xc0", "\xed\x9f\xc0"},
		// lastIndexTests
		{"foo", "f"}, {"oofofoofooo", "foo"}, {"barfoobarfoo", "foo"},
		{"abcABCabc", "a"},
	} {
		cs = append(cs, strsearchCase{t[0], t[1]})
	}
	return cs
}

// goSplitCases are Go's strings_test.go splittests with n < 0 (strings.Split)
// and n == 0 skipped (Str.split has no n). Go's strings tests are
// Copyright 2009 The Go Authors, BSD-style license.
func goSplitCases() []strsearchCase {
	var cs []strsearchCase
	for _, t := range [][2]string{
		{"", ""}, {"abcd", ""}, {"abcd", ""}, {"abcd", ""}, {"☺☻☹", ""},
		{"☺☻☹", ""}, {"☺☻☹", ""}, {"☺�☹", ""},
		{"abcd", "a"}, {"abcd", "z"}, {"1,2,3,4", ","}, {"1....2....3....4", "..."},
		{"☺☻☹", "☹"}, {"☺☻☹", "~"}, {"1 2 3 4", " "}, {"1 2", " "},
		{"", "T"}, {"\xff-\xff", ""}, {"\xff-\xff", "-"},
	} {
		cs = append(cs, strsearchCase{t[0], t[1]})
	}
	return cs
}

// strsearchReplace is one (s, old, new) input of Str.replace.
type strsearchReplace struct{ s, old, new string }

// goReplaceCases are Go's strings_test.go ReplaceTests with n == -1
// (strings.ReplaceAll). Go's strings tests are Copyright 2009 The Go
// Authors, BSD-style license.
func goReplaceCases() []strsearchReplace {
	var cs []strsearchReplace
	for _, t := range [][3]string{
		{"hello", "l", "L"}, {"hello", "x", "X"}, {"", "x", "X"},
		{"radar", "r", "<r>"}, {"", "", "<>"}, {"banana", "a", "<>"},
		{"banana", "an", "<>"}, {"banana", "ana", "<>"}, {"banana", "", "<>"},
		{"banana", "a", "a"}, {"☺☻☹", "", "<>"},
	} {
		cs = append(cs, strsearchReplace{t[0], t[1], t[2]})
	}
	return cs
}

// strsearchAlphabet has no U+0000 (the list separator) and no U+001F (the
// record separator): invalid bytes read as U+FFFD (decision 0259), and the
// other entries cover one byte, two byte and multi byte separators.
var strsearchAlphabet = []string{
	"a", "b", "c", "A", "0", "-", ",", ".", "/", " ", " ", "\t", "\n", "\r",
	"é", "日", "☺", "\u0085", " ", " ", " ", " ", "　", "᠎", "​", "\x80", "\xff",
}

func strsearchGen(r *rand.Rand, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(strsearchAlphabet[r.Intn(len(strsearchAlphabet))])
	}
	return b.String()
}

var strsearchLens = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 15, 16, 17, 20, 31, 44, 70}

func strsearchLen(r *rand.Rand) int { return strsearchLens[r.Intn(len(strsearchLens))] }

// strsearchRandomIndex is a random (s, sub): sub is unrelated, a prefix or a
// suffix of s, with one byte changed in a quarter of those, and its length
// covers the one byte, word and multi word searches.
func strsearchRandomIndex(r *rand.Rand) strsearchCase {
	s := strsearchGen(r, strsearchLen(r))
	p := strsearchGen(r, r.Intn(5))
	switch r.Intn(4) {
	case 0, 1:
		if len(s) > 0 {
			n := r.Intn(len(s) + 1)
			if r.Intn(2) == 0 {
				p = s[:n]
			} else {
				p = s[len(s)-n:]
			}
		}
		if len(p) > 0 && r.Intn(4) == 0 {
			b := []byte(p)
			b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
			p = string(b)
		}
	}
	return strsearchCase{s, p}
}

// strsearchRandomSplit is a random (s, sep): sep is empty, one byte or a
// piece of s, so the empty, one byte and multi byte paths and a separator
// at each end are covered.
func strsearchRandomSplit(r *rand.Rand) strsearchCase {
	s := strsearchGen(r, strsearchLen(r))
	var sep string
	switch r.Intn(5) {
	case 0:
		sep = ""
	case 1, 2:
		sep = strsearchGen(r, 1+r.Intn(3))
	default:
		sep = strsearchGen(r, 1+r.Intn(2))
		if len(s) > 0 && r.Intn(2) == 0 {
			n := 1 + r.Intn(len(s))
			start := r.Intn(len(s) - n + 1)
			sep = s[start : start+n]
		}
	}
	return strsearchCase{s, sep}
}

// strsearchRandomReplace is a random (s, old, new): old is unrelated, empty
// or a piece of s, so replace all, no match and the empty old insertion
// before each UTF-8 sequence are covered.
func strsearchRandomReplace(r *rand.Rand) strsearchReplace {
	s := strsearchGen(r, strsearchLen(r))
	old := strsearchGen(r, r.Intn(4))
	switch r.Intn(4) {
	case 0:
		old = ""
	case 1:
		if len(s) > 0 {
			n := 1 + r.Intn(len(s))
			start := r.Intn(len(s) - n + 1)
			old = s[start : start+n]
		}
	}
	return strsearchReplace{s, old, strsearchGen(r, r.Intn(4))}
}

// strsearchOracleIndex is Go's Index, LastIndex and Contains as one
// "\x1f" separated record, None as -1.
func strsearchOracleIndex(c strsearchCase) string {
	return fmt.Sprintf("%d\x1f%d\x1f%v", strings.Index(c.s, c.sep), strings.LastIndex(c.s, c.sep), strings.Contains(c.s, c.sep))
}

// strsearchOracleSplit is Go's Split(s, sep) as its length and its pieces
// joined by U+0000, so an empty list differs from one empty piece.
func strsearchOracleSplit(c strsearchCase) string {
	parts := strings.Split(c.s, c.sep)
	return fmt.Sprintf("%d\x1f%s", len(parts), strings.Join(parts, "\x00"))
}

// strsearchOracleJoin is Go's Join(parts, sep).
func strsearchOracleJoin(parts []string, sep string) string { return strings.Join(parts, sep) }

// strsearchText maps the inputs as std/rt maps them: an invalid byte reads
// as U+FFFD (decision 0259), so Go and Xo see the same Str.
func strsearchText(c strsearchCase) strsearchCase { return strsearchCase{textOf(c.s), textOf(c.sep)} }

func strsearchCorpus() string {
	r := rand.New(rand.NewSource(31))
	var o strings.Builder
	o.WriteString(header("strings.Index, LastIndex, Contains, Split, ReplaceAll and Join on the inputs of Go's strings_test.go (indexTests, lastIndexTests, splittests, ReplaceTests) and on seeded random inputs, against Str.find, rfind, contains, split, replace and List[Str].join (Go's strings tests are Copyright 2009 The Go Authors, BSD-style license)"))

	// index, rfind and contains
	idx := goIndexCases()
	for i := 0; i < 5000; i++ {
		idx = append(idx, strsearchRandomIndex(r))
	}
	o.WriteString("/// (s, sub, Go's Index, LastIndex and Contains joined by U+001F; None as -1)\nconst INDEX_CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range idx {
		c = strsearchText(c)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(c.s), xq(c.sep), xq(strsearchOracleIndex(c)))
	}
	o.WriteString("]\n")

	// split
	sp := goSplitCases()
	for i := 0; i < 3000; i++ {
		sp = append(sp, strsearchRandomSplit(r))
	}
	o.WriteString("\n/// (s, sep, Go's Split as its length and its pieces joined by U+0000, joined by U+001F)\nconst SPLIT_CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range sp {
		c = strsearchText(c)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(c.s), xq(c.sep), xq(strsearchOracleSplit(c)))
	}
	o.WriteString("]\n")

	// replace
	rp := goReplaceCases()
	for i := 0; i < 3000; i++ {
		rp = append(rp, strsearchRandomReplace(r))
	}
	o.WriteString("\n/// (s, old, new, Go's ReplaceAll)\nconst REPLACE_CASES: List[(Str, Str, Str, Str)] = [\n")
	for _, c := range rp {
		in := strsearchText(strsearchCase{c.s, c.old})
		nt := textOf(c.new)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xq(in.s), xq(in.sep), xq(nt), xq(strings.ReplaceAll(in.s, in.sep, nt)))
	}
	o.WriteString("]\n")

	// join
	o.WriteString("\n/// (parts, sep, Go's Join)\nconst JOIN_CASES: List[(List[Str], Str, Str)] = [\n")
	for i := 0; i < 1500; i++ {
		n := r.Intn(9)
		parts := make([]string, n)
		for j := range parts {
			parts[j] = textOf(strsearchGen(r, r.Intn(6)))
		}
		sep := textOf(strsearchGen(r, r.Intn(3)))
		var lits strings.Builder
		lits.WriteString("[")
		for j, p := range parts {
			if j > 0 {
				lits.WriteString(", ")
			}
			lits.WriteString(xq(p))
		}
		lits.WriteString("]")
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", lits.String(), xq(sep), xq(strsearchOracleJoin(parts, sep)))
	}
	o.WriteString("]\n")

	o.WriteString(strsearchTests)
	return o.String()
}

const strsearchTests = `
fn idx_of(s: Str, sub: Str) -> Str {
  "${s.find(sub) ?? -1}\u{1f}${s.rfind(sub) ?? -1}\u{1f}${s.contains(sub)}"
}

fn list_of(xs: List[Str]) -> Str {
  "${xs.len()}\u{1f}${xs.join("\u{0}")}"
}

fn show3(bad: List[(Str, Str, Str, Str)]) -> List[(Str, Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "find, rfind and contains are Go's Index, LastIndex and Contains" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sub, want) in INDEX_CASES {
    let got = idx_of(s, sub)
    if got != want {
      bad.push((s, sub, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "find, rfind and contains on a view of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sub, want) in INDEX_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = idx_of(view, sub)
    if got != want {
      bad.push((s, sub, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "split is Go's Split" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sep, want) in SPLIT_CASES {
    let got = list_of(s.split(sep))
    if got != want {
      bad.push((s, sep, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "split on a view of a longer text, and join puts the pieces back" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sep, want) in SPLIT_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = list_of(view.split(sep))
    if got != want {
      bad.push((s, sep, got, want))
    }
    let back = view.split(sep).join(sep)
    if back != view {
      bad.push((s, sep, back, view))
    }
  }
  expect(show3(bad)) == []
}

test "replace is Go's ReplaceAll" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, old, new, want) in REPLACE_CASES {
    let got = s.replace(old, new)
    if got != want {
      bad.push((s, old, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "replace on a view of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, old, new, want) in REPLACE_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = view.replace(old, new)
    if got != want {
      bad.push((s, old, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "join is Go's Join" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (parts, sep, want) in JOIN_CASES {
    let got = parts.join(sep)
    if got != want {
      bad.push((parts.join("\u{1f}"), sep, got, want))
    }
  }
  expect(show3(bad)) == []
}
`
