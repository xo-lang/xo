package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// cutCase and its tables are Go's strings_test.go cutTests, cutPrefixTests and
// cutSuffixTests merged as (s, sep) inputs of Str.cut, Str.cut_prefix and
// Str.cut_suffix; the results come from Go's strings.Cut, CutPrefix and
// CutSuffix. Go's strings tests are Copyright 2009 The Go Authors,
// BSD-style license.

// cutCase is one (s, sep) input of the three cut methods.
type cutCase struct{ s, sep string }

func goCutCases() []cutCase {
	return []cutCase{
		{"abc", "b"}, {"abc", "a"}, {"abc", "c"}, {"abc", "abc"},
		{"abc", ""}, {"abc", "d"}, {"", "d"}, {"", ""},
	}
}

// cutPrefixCases and cutSuffixCases are Go's cutPrefixTests and
// cutSuffixTests; they share the (s, sep) shape of a cut.
func goCutPrefixCases() []cutCase {
	return []cutCase{
		{"abc", "a"}, {"abc", "abc"}, {"abc", ""},
		{"abc", "d"}, {"", "d"}, {"", ""},
	}
}

func goCutSuffixCases() []cutCase {
	return []cutCase{
		{"abc", "bc"}, {"abc", "abc"}, {"abc", ""},
		{"abc", "d"}, {"", "d"}, {"", ""},
	}
}

// cutAlphabet is the random input alphabet: ASCII, a byte that is not a
// separator, separators, and two and three byte runes, so a sep can hold a
// whole rune or begin inside one.
var cutAlphabet = []string{"a", "b", "c", "A", "0", ".", "/", "-", "_", " ", "é", "ß", "☺", "Σ", "語"}

var cutLens = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 15, 16, 17, 20, 31}

func cutLen(r *rand.Rand) int { return cutLens[r.Intn(len(cutLens))] }

// cutRandomText is a random text of the alphabet.
func cutRandomText(r *rand.Rand) string {
	var b strings.Builder
	for i := 0; i < cutLen(r); i++ {
		b.WriteString(cutAlphabet[r.Intn(len(cutAlphabet))])
	}
	return b.String()
}

// cutRandomRunePiece is a rune aligned piece of s, or s itself, or "".
func cutRandomRunePiece(r *rand.Rand, s string) string {
	rs := []rune(s)
	if len(rs) == 0 {
		return ""
	}
	lo := r.Intn(len(rs) + 1)
	hi := lo + r.Intn(len(rs)-lo+1)
	return string(rs[lo:hi])
}

// cutRandomPair is a random (text, sep): the sep is empty, one alphabet
// element, unrelated text, or a rune aligned piece of the text (so most
// cases are found, and a few are not).
func cutRandomPair(r *rand.Rand) cutCase {
	s := cutRandomText(r)
	var sep string
	switch r.Intn(4) {
	case 0:
		sep = ""
	case 1:
		sep = cutAlphabet[r.Intn(len(cutAlphabet))]
	case 2:
		sep = cutRandomRunePiece(r, s)
	default:
		sep = cutRandomText(r)
	}
	return cutCase{s, sep}
}

// cutRandomAffix is a random (text, affix) for cut_prefix and cut_suffix:
// the affix is a rune aligned piece of the text half the time, and an
// unrelated one otherwise, so both hits and misses occur.
func cutRandomAffix(r *rand.Rand) cutCase {
	s := cutRandomText(r)
	var sep string
	if r.Intn(2) == 0 {
		sep = cutRandomRunePiece(r, s)
	} else {
		sep = cutRandomText(r)
	}
	return cutCase{s, sep}
}

// cutBoundaryCases are the byte-length boundaries of the separator compare
// (2026-10-25): 0, 1, 7, 8 and 9 bytes, at the start of the text, at its
// end, in its middle, and absent from it. 8 is where the compare switches
// from the overlapping short loads to two words, and the cut methods
// compare a prefix or a suffix directly.
func cutBoundaryCases() []cutCase {
	var out []cutCase
	for _, n := range []int{0, 1, 7, 8, 9} {
		sep := strings.Repeat("a", n)
		out = append(out,
			cutCase{sep + "XYZ", sep},      // at the start
			cutCase{"XYZ" + sep, sep},      // at the end
			cutCase{"XY" + sep + "Z", sep}, // in the middle
			cutCase{"XYZ", sep},            // absent
		)
	}
	return out
}

// cutWant is Go's Cut as a corpus result: none, or the two pieces joined
// by U+001F after the marker.
func cutWant(s, sep string) string {
	before, after, ok := strings.Cut(s, sep)
	if !ok {
		return "none"
	}
	return "some\x1f" + before + "\x1f" + after
}

// cutAffixWant is Go's CutPrefix or CutSuffix as a corpus result.
func cutAffixWant(rest string, ok bool) string {
	if !ok {
		return "none"
	}
	return "some\x1f" + rest
}

func cutCorpus() string {
	var o strings.Builder
	o.WriteString(header("strings.Cut, CutPrefix and CutSuffix on the inputs of Go's strings_test.go (cutTests, cutPrefixTests, cutSuffixTests) and on seeded random inputs, against Str.cut, Str.cut_prefix and Str.cut_suffix (Go's strings tests are Copyright 2009 The Go Authors, BSD-style license)"))
	r := rand.New(rand.NewSource(47))

	// cut
	cs := append(goCutCases(), cutBoundaryCases()...)
	for i := 0; i < 2000; i++ {
		cs = append(cs, cutRandomPair(r))
	}
	o.WriteString("\n/// (s, sep, Go's Cut: none, or some and the two pieces, joined by U+001F)\nconst CUT_CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range cs {
		s, sep := textOf(c.s), textOf(c.sep)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(s), xq(sep), xq(cutWant(s, sep)))
	}
	o.WriteString("]\n")

	// cut_prefix, on its own seed so cut's random inputs stay fixed
	ps := append(goCutPrefixCases(), cutBoundaryCases()...)
	rp := rand.New(rand.NewSource(53))
	for i := 0; i < 2000; i++ {
		ps = append(ps, cutRandomAffix(rp))
	}
	o.WriteString("\n/// (s, prefix, Go's CutPrefix: none, or some and the rest, joined by U+001F)\nconst CUT_PREFIX_CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range ps {
		s, prefix := textOf(c.s), textOf(c.sep)
		rest, ok := strings.CutPrefix(s, prefix)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(s), xq(prefix), xq(cutAffixWant(rest, ok)))
	}
	o.WriteString("]\n")

	// cut_suffix
	ss := append(goCutSuffixCases(), cutBoundaryCases()...)
	rs := rand.New(rand.NewSource(59))
	for i := 0; i < 2000; i++ {
		ss = append(ss, cutRandomAffix(rs))
	}
	o.WriteString("\n/// (s, suffix, Go's CutSuffix: none, or some and the rest, joined by U+001F)\nconst CUT_SUFFIX_CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range ss {
		s, suffix := textOf(c.s), textOf(c.sep)
		before, ok := strings.CutSuffix(s, suffix)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(s), xq(suffix), xq(cutAffixWant(before, ok)))
	}
	o.WriteString("]\n")

	o.WriteString(cutTests)
	return o.String()
}

const cutTests = `
fn show_cut(r: (Str, Str)?) -> Str {
  match r {
    None => "none"
    Some((before, after)) => "some\u{1f}${before}\u{1f}${after}"
  }
}

fn show_some(r: Str?) -> Str {
  match r {
    None => "none"
    Some(x) => "some\u{1f}${x}"
  }
}

fn show3(bad: List[(Str, Str, Str, Str)]) -> List[(Str, Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "cut is Go's Cut" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sep, want) in CUT_CASES {
    let got = show_cut(s.cut(sep))
    if got != want {
      bad.push((s, sep, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "cut on a view of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sep, want) in CUT_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = show_cut(view.cut(sep))
    if got != want {
      bad.push((s, sep, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "cut_prefix is Go's CutPrefix" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, prefix, want) in CUT_PREFIX_CASES {
    let got = show_some(s.cut_prefix(prefix))
    if got != want {
      bad.push((s, prefix, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "cut_prefix on a view of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, prefix, want) in CUT_PREFIX_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = show_some(view.cut_prefix(prefix))
    if got != want {
      bad.push((s, prefix, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "cut_suffix is Go's CutSuffix" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, suffix, want) in CUT_SUFFIX_CASES {
    let got = show_some(s.cut_suffix(suffix))
    if got != want {
      bad.push((s, suffix, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "cut_suffix on a view of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, suffix, want) in CUT_SUFFIX_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = show_some(view.cut_suffix(suffix))
    if got != want {
      bad.push((s, suffix, got, want))
    }
  }
  expect(show3(bad)) == []
}
`
