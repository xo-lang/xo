package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"slices"
	"strings"
)

// strsearchCase is one (s, sep) input of the Str search and split methods.
type strsearchCase struct{ s, sep string }

// strsearchSplitN is one (s, sep, n) input of Str.split_n, split_after and
// split_after_n.
type strsearchSplitN struct {
	s, sep string
	n      int
}

// The API these corpora cover, as std/prelude.xo declares it: Str.find
// (strings.Index), Str.rfind (strings.LastIndex), Str.contains
// (strings.Contains), Str.count (strings.Count), Str.find_any and
// Str.rfind_any (strings.IndexAny, LastIndexAny), Str.contains_any
// (strings.ContainsAny), Str.contains_rune (strings.ContainsRune),
// Str.split (strings.Split), Str.replace (strings.ReplaceAll) and
// List[Str].join (strings.Join). Go's n of Replace has no Str method, so
// that table is not here; replace_all is Str.replace, which always
// replaces every match.

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

// goCountCases are Go's strings_test.go CountTests as (s, sub) inputs; the
// count itself comes from strings.Count. Go's strings tests are Copyright
// 2009 The Go Authors, BSD-style license.
func goCountCases() []strsearchCase {
	var cs []strsearchCase
	for _, t := range [][2]string{
		{"", ""}, {"", "notempty"}, {"notempty", ""},
		{"smaller", "not smaller"}, {"12345678987654321", "6"},
		{"611161116", "6"}, {"notequal", "NotEqual"}, {"equal", "equal"},
		{"abc1231231123q", "123"}, {"11111", "11"},
	} {
		cs = append(cs, strsearchCase{t[0], t[1]})
	}
	return cs
}

// splitTestTable is Go's strings_test.go SplitTest constants, shared by
// splittests and splitaftertests.
const (
	stABCD   = "abcd"
	stFaces  = "☺☻☹"
	stCommas = "1,2,3,4"
	stDots   = "1....2....3....4"
)

// goSplitNTests are Go's strings_test.go splittests (strings.SplitN) and
// splitaftertests (strings.SplitAfterN) as (s, sep, n) inputs; each is
// checked against the table's pieces here, so the port is verified when the
// corpus is generated. Go's strings tests are Copyright 2009 The Go Authors,
// BSD-style license.
func goSplitNTests() []strsearchSplitN {
	var cs []strsearchSplitN
	type splitTest struct {
		s, sep string
		n      int
		a      []string
	}
	for _, t := range []splitTest{
		{"", "", -1, []string{}},
		{stABCD, "", 2, []string{"a", "bcd"}},
		{stABCD, "", 4, []string{"a", "b", "c", "d"}},
		{stABCD, "", -1, []string{"a", "b", "c", "d"}},
		{stFaces, "", -1, []string{"\u263a", "\u263b", "\u2639"}},
		{stFaces, "", 3, []string{"\u263a", "\u263b", "\u2639"}},
		{stFaces, "", 17, []string{"\u263a", "\u263b", "\u2639"}},
		{"\u263a\ufffd\u2639", "", -1, []string{"\u263a", "\ufffd", "\u2639"}},
		{stABCD, "a", 0, nil},
		{stABCD, "a", -1, []string{"", "bcd"}},
		{stABCD, "z", -1, []string{"abcd"}},
		{stCommas, ",", -1, []string{"1", "2", "3", "4"}},
		{stDots, "...", -1, []string{"1", ".2", ".3", ".4"}},
		{stFaces, "\u2639", -1, []string{"\u263a\u263b", ""}},
		{stFaces, "~", -1, []string{stFaces}},
		{"1 2 3 4", " ", 3, []string{"1", "2", "3 4"}},
		{"1 2", " ", 3, []string{"1", "2"}},
		{"", "T", math.MaxInt / 4, []string{""}},
		{"\xff-\xff", "", -1, []string{"\xff", "-", "\xff"}},
		{"\xff-\xff", "-", -1, []string{"\xff", "\xff"}},
	} {
		if got := strings.SplitN(t.s, t.sep, t.n); !slices.Equal(got, t.a) {
			fmt.Fprintf(os.Stderr, "stdcorpus: strings.SplitN(%q, %q, %d) = %q, Go's test says %q\n", t.s, t.sep, t.n, got, t.a)
			os.Exit(1)
		}
		cs = append(cs, strsearchSplitN{t.s, t.sep, t.n})
	}
	for _, t := range []splitTest{
		{stABCD, "a", -1, []string{"a", "bcd"}},
		{stABCD, "z", -1, []string{"abcd"}},
		{stABCD, "", -1, []string{"a", "b", "c", "d"}},
		{stCommas, ",", -1, []string{"1,", "2,", "3,", "4"}},
		{stDots, "...", -1, []string{"1...", ".2...", ".3...", ".4"}},
		{stFaces, "\u2639", -1, []string{"\u263a\u263b\u2639", ""}},
		{stFaces, "~", -1, []string{stFaces}},
		{stFaces, "", -1, []string{"\u263a", "\u263b", "\u2639"}},
		{"1 2 3 4", " ", 3, []string{"1 ", "2 ", "3 4"}},
		{"1 2 3", " ", 3, []string{"1 ", "2 ", "3"}},
		{"1 2", " ", 3, []string{"1 ", "2"}},
		{"123", "", 2, []string{"1", "23"}},
		{"123", "", 17, []string{"1", "2", "3"}},
	} {
		if got := strings.SplitAfterN(t.s, t.sep, t.n); !slices.Equal(got, t.a) {
			fmt.Fprintf(os.Stderr, "stdcorpus: strings.SplitAfterN(%q, %q, %d) = %q, Go's test says %q\n", t.s, t.sep, t.n, got, t.a)
			os.Exit(1)
		}
		cs = append(cs, strsearchSplitN{t.s, t.sep, t.n})
	}
	return cs
}

// strsearchRandomSplitN is a random (s, sep, n): sep is empty, one byte, a
// piece of s or unrelated, and n is below 0, 0, 1 to 5 or above the piece
// count.
func strsearchRandomSplitN(r *rand.Rand) strsearchSplitN {
	c := strsearchCase{s: strsearchGen(r, strsearchLen(r))}
	switch r.Intn(4) {
	case 0:
		c.sep = ""
	case 1:
		c.sep = strsearchGen(r, 1)
	case 2:
		if c.s != "" {
			n := 1 + r.Intn(len(c.s))
			st := r.Intn(len(c.s) - n + 1)
			c.sep = c.s[st : st+n]
		} else {
			c.sep = "-"
		}
	default:
		c.sep = strsearchGen(r, 1+r.Intn(3))
	}
	ns := []int{-3, -1, 0, 1, 2, 3, 4, 5, 9, 40}
	return strsearchSplitN{c.s, c.sep, ns[r.Intn(len(ns))]}
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

// goFieldsCases are Go's strings_test.go fieldstests (FieldsTest); only
// the input is used, since Str.fields takes no argument. Go's strings tests
// are Copyright 2009 The Go Authors, BSD-style license.
func goFieldsCases() []string {
	return []string{
		"", " ", " \t ", "\u2000", "  abc  ", "1 2 3 4", "1  2  3  4",
		"1\t\t2\t\t3\t4", "1\u20002\u20013\u20024", "\u2000\u2001\u2002",
		"\n™\t™\n", "\n\u20001™2\u2000 \u2001™",
		"\n1\uFFFD \uFFFD2\u20003\uFFFD4", "1\xFF\u2000\xFF2\xFF \xFF",
		"☺☻☹",
	}
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

// strsearchDots is Go's dots in strings_test.go (dots + dots + dots is one
// of the inputs).
const strsearchDots = "1....2....3....4"

// goAnyCases are Go's strings_test.go indexAnyTests and lastIndexAnyTests as
// (s, chars) inputs for Str.find_any and Str.rfind_any. Each table's own
// offset is checked against Go here, and against Go's answer on the text as
// std/rt reads it (an invalid byte as U+FFFD, decision 0259): an offset after
// an invalid byte moves (Go's 7 for "012\\x80bcb\\x80210" is 9 as Xo reads
// it), and the note tells which, since the corpus records Go's answer on the
// converted text. The two tables have different inputs ("aRegExp*" against
// "a.RegExp*"), and each pair is kept as its table states it. Go's strings
// tests are Copyright 2009 The Go Authors, BSD-style license.
func goAnyCases() []strsearchCase {
	type anyTest struct {
		s, chars string
		want     int
	}
	var cs []strsearchCase
	add := func(t anyTest, last bool) {
		name, f := "IndexAny", strings.IndexAny
		if last {
			name, f = "LastIndexAny", strings.LastIndexAny
		}
		if got := f(t.s, t.chars); got != t.want {
			fmt.Fprintf(os.Stderr, "stdcorpus: strings.%s(%q, %q) = %d, Go's test says %d\n", name, t.s, t.chars, got, t.want)
			os.Exit(1)
		}
		if a, b := textOf(t.s), textOf(t.chars); f(a, b) != t.want {
			// an invalid byte reads as U+FFFD (decision 0259), three
			// bytes where it was one, so an offset after it moves: the
			// corpus records Go's answer on the text as Xo holds it
			fmt.Fprintf(os.Stderr, "stdcorpus: note: %s(%q, %q) = %d on the text as Xo reads it (%q, %q), Go's test says %d\n", name, t.s, t.chars, f(a, b), a, b, t.want)
		}
		cs = append(cs, strsearchCase{t.s, t.chars})
	}
	for _, t := range []anyTest{
		{"", "", -1},
		{"", "a", -1},
		{"", "abc", -1},
		{"a", "", -1},
		{"a", "a", 0},
		{"\x80", "\xffb", 0},
		{"aaa", "a", 0},
		{"abc", "xyz", -1},
		{"abc", "xcz", 2},
		{"ab☺c", "x☺yz", 2},
		{"a☺b☻c☹d", "cx", len("a☺b☻")},
		{"a☺b☻c☹d", "uvw☻xyz", len("a☺b")},
		{"aRegExp*", ".(|)*+?^$[]", 7},
		{strsearchDots + strsearchDots + strsearchDots, " ", -1},
		{"012abcba210", "\xffb", 4},
		{"012\x80bcb\x80210", "\xffb", 3},
		{"0123456\xcf\x80abc", "\xcfb\x80", 10},
	} {
		add(t, false)
	}
	for _, t := range []anyTest{
		{"", "", -1},
		{"", "a", -1},
		{"", "abc", -1},
		{"a", "", -1},
		{"a", "a", 0},
		{"\x80", "\xffb", 0},
		{"aaa", "a", 2},
		{"abc", "xyz", -1},
		{"abc", "ab", 1},
		{"ab☺c", "x☺yz", 2},
		{"a☺b☻c☹d", "cx", len("a☺b☻")},
		{"a☺b☻c☹d", "uvw☻xyz", len("a☺b")},
		{"a.RegExp*", ".(|)*+?^$[]", 8},
		{strsearchDots + strsearchDots + strsearchDots, " ", -1},
		{"012abcba210", "\xffb", 6},
		{"012\x80bcb\x80210", "\xffb", 7},
		{"0123456\xcf\x80abc", "\xcfb\x80", 10},
	} {
		add(t, true)
	}
	return cs
}

// goContainsAnyCases are Go's strings_test.go ContainsAnyTests.
func goContainsAnyCases() []strsearchCase {
	var cs []strsearchCase
	for _, t := range []struct {
		s, chars string
		want     bool
	}{
		{"", "", false},
		{"", "a", false},
		{"", "abc", false},
		{"a", "", false},
		{"a", "a", true},
		{"aaa", "a", true},
		{"abc", "xyz", false},
		{"abc", "xcz", true},
		{"a☺b☻c☹d", "uvw☻xyz", true},
		{"aRegExp*", ".(|)*+?^$[]", true},
		{strsearchDots + strsearchDots + strsearchDots, " ", false},
	} {
		if got := strings.ContainsAny(t.s, t.chars); got != t.want {
			fmt.Fprintf(os.Stderr, "stdcorpus: strings.ContainsAny(%q, %q) = %v, Go's test says %v\n", t.s, t.chars, got, t.want)
			os.Exit(1)
		}
		cs = append(cs, strsearchCase{t.s, t.chars})
	}
	return cs
}

// strsearchRune is one (s, r) input of Str.contains_rune.
type strsearchRune struct {
	s string
	r rune
}

// goContainsRuneCases are Go's strings_test.go ContainsRuneTests.
func goContainsRuneCases() []strsearchRune {
	var cs []strsearchRune
	for _, t := range []struct {
		s    string
		r    rune
		want bool
	}{
		{"", 'a', false},
		{"a", 'a', true},
		{"aaa", 'a', true},
		{"abc", 'y', false},
		{"abc", 'c', true},
		{"a☺b☻c☹d", 'x', false},
		{"a☺b☻c☹d", '☻', true},
		{"aRegExp*", '*', true},
	} {
		if got := strings.ContainsRune(t.s, t.r); got != t.want {
			fmt.Fprintf(os.Stderr, "stdcorpus: strings.ContainsRune(%q, %q) = %v, Go's test says %v\n", t.s, t.r, got, t.want)
			os.Exit(1)
		}
		cs = append(cs, strsearchRune{t.s, t.r})
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

// strsearchFieldAlphabet covers every class of unicode.IsSpace, the ASCII
// set and U+0085, U+00A0, U+1680, U+2000 to U+200A, U+2028, U+2029, U+202F,
// U+205F and U+3000, beside non-space text and invalid bytes (U+FFFD in a
// Str, decision 0259). U+0000 and U+001F are left out: the list and record
// separators.
var strsearchFieldAlphabet = []string{
	"a", "b", "c", "A", "0", "-", ",", ".", "/", "é", "日", "☺", "\x80", "\xff",
	"\t", "\n", "\v", "\f", "\r", " ",
	"\u0085", "\u00a0", "\u1680", "\u2000", "\u2001", "\u2002", "\u2003",
	"\u2004", "\u2005", "\u2006", "\u2007", "\u2008", "\u2009", "\u200a",
	"\u2028", "\u2029", "\u202f", "\u205f", "\u3000",
}

// strsearchRandomFields is a random text over strsearchFieldAlphabet, so runs
// of ASCII space, runs of non-ASCII space, a leading or trailing run, and
// non-space text between them all occur.
func strsearchRandomFields(r *rand.Rand) string {
	var b strings.Builder
	n := strsearchLen(r)
	if r.Intn(4) == 0 {
		b.WriteString("\u2000")
	}
	for i := 0; i < n; i++ {
		b.WriteString(strsearchFieldAlphabet[r.Intn(len(strsearchFieldAlphabet))])
	}
	if r.Intn(4) == 0 {
		b.WriteString("\u3000")
	}
	return b.String()
}

// strsearchRandomCount is a random (s, sub): sub is empty, one byte, a
// longer unrelated text, or a piece of s, so overlapping copies ("11111"
// and "11") and a multi byte piece occur.
func strsearchRandomCount(r *rand.Rand) strsearchCase {
	s := strsearchGen(r, strsearchLen(r))
	var sub string
	switch r.Intn(5) {
	case 0:
		sub = ""
	case 1:
		sub = strsearchGen(r, 1)
	case 2:
		sub = strsearchGen(r, 2+r.Intn(3))
	default:
		if len(s) > 0 {
			n := 1 + r.Intn(len(s))
			start := r.Intn(len(s) - n + 1)
			sub = s[start : start+n]
		} else {
			sub = strsearchGen(r, 1)
		}
	}
	return strsearchCase{s, sub}
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

// strsearchRandomAny is a random (s, chars): chars is empty, one byte, a
// short ASCII set, a multi byte set, a piece of s, or holds U+FFFD, so Go's
// one byte, word set and rune by rune paths all occur.
func strsearchRandomAny(r *rand.Rand) strsearchCase {
	s := strsearchGen(r, strsearchLen(r))
	var chars string
	switch r.Intn(6) {
	case 0:
		chars = ""
	case 1:
		chars = strsearchGen(r, 1)
	case 2:
		var b strings.Builder
		for i := 0; i < 1+r.Intn(5); i++ {
			b.WriteByte(byte(0x20 + r.Intn(0x5f)))
		}
		chars = b.String()
	case 3:
		chars = strsearchGen(r, 1+r.Intn(3))
	case 4:
		if len(s) > 0 {
			n := 1 + r.Intn(len(s))
			at := r.Intn(len(s) - n + 1)
			chars = s[at : at+n]
		} else {
			chars = "a"
		}
	default:
		chars = "\uFFFD" + strsearchGen(r, r.Intn(2))
	}
	return strsearchCase{s, chars}
}

// strsearchRandomRune is a random (s, r): r is a rune of s, one of the
// alphabet's, U+FFFD, or one s need not hold, so the found and the missing
// case, the ASCII and the multi byte path and RuneError all occur.
func strsearchRandomRune(r *rand.Rand) strsearchRune {
	s := strsearchGen(r, strsearchLen(r))
	var cp rune
	switch r.Intn(4) {
	case 0, 1:
		rs := []rune(textOf(s))
		if len(rs) > 0 {
			cp = rs[r.Intn(len(rs))]
		} else {
			cp = 'a'
		}
	case 2:
		cp = []rune(textOf(strsearchAlphabet[r.Intn(len(strsearchAlphabet))]))[0]
	default:
		cp = []rune("aA0-\u00e9\u65e5\u263a\uFFFD\u2000")[r.Intn(9)]
	}
	return strsearchRune{s, cp}
}

// strsearchOracleIndex is Go's Index, LastIndex and Contains as one
// "\x1f" separated record, None as -1.
func strsearchOracleIndex(c strsearchCase) string {
	return fmt.Sprintf("%d\x1f%d\x1f%v", strings.Index(c.s, c.sep), strings.LastIndex(c.s, c.sep), strings.Contains(c.s, c.sep))
}

// strsearchOracleAny is Go's IndexAny, LastIndexAny and ContainsAny as one
// "\x1f" separated record, None as -1.
func strsearchOracleAny(c strsearchCase) string {
	return fmt.Sprintf("%d\x1f%d\x1f%v", strings.IndexAny(c.s, c.sep), strings.LastIndexAny(c.s, c.sep), strings.ContainsAny(c.s, c.sep))
}

// strsearchOracleSplit is Go's Split(s, sep) as its length and its pieces
// joined by U+0000, so an empty list differs from one empty piece.
func strsearchOracleSplit(c strsearchCase) string { return strsearchList(strings.Split(c.s, c.sep)) }

// strsearchOracleFields is Go's Fields(s) in strsearchList's form.
func strsearchOracleFields(s string) string { return strsearchList(strings.Fields(s)) }

// strsearchList is parts as its length and its pieces joined by U+0000, so
// an empty list differs from one empty piece.
func strsearchList(parts []string) string {
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
	o.WriteString(header("strings.Index, LastIndex, Contains, IndexAny, LastIndexAny, ContainsAny, ContainsRune, Split, SplitN, SplitAfterN, Fields, ReplaceAll and Join on the inputs of Go's strings_test.go (indexTests, lastIndexTests, indexAnyTests, lastIndexAnyTests, ContainsAnyTests, ContainsRuneTests, splittests, splitaftertests, fieldstests, ReplaceTests) and on seeded random inputs, against Str.find, rfind, contains, find_any, rfind_any, contains_any, contains_rune, split, split_n, split_after, split_after_n, fields, replace and List[Str].join, and strings.Count (countTests) against Str.count (Go's strings tests are Copyright 2009 The Go Authors, BSD-style license)"))

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

	// fields (its own seed, so the later corpora keep their random inputs)
	fd := goFieldsCases()
	rf := rand.New(rand.NewSource(37))
	for i := 0; i < 3000; i++ {
		fd = append(fd, strsearchRandomFields(rf))
	}
	o.WriteString("\n/// (s, Go's Fields as its length and its pieces joined by U+0000, joined by U+001F)\nconst FIELDS_CASES: List[(Str, Str)] = [\n")
	for _, s := range fd {
		s = textOf(s)
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(strsearchOracleFields(s)))
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

	// count (its own seed, after every section that shares r, so their
	// random inputs stay identical)
	ct := goCountCases()
	rc := rand.New(rand.NewSource(41))
	for i := 0; i < 3000; i++ {
		ct = append(ct, strsearchRandomCount(rc))
	}
	o.WriteString("\n/// (s, sub, Go's Count)\nconst COUNT_CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range ct {
		c = strsearchText(c)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(c.s), xq(c.sep), xq(fmt.Sprintf("%d", strings.Count(c.s, c.sep))))
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (s, chars, Go's IndexAny and LastIndexAny and ContainsAny joined by U+001F; None as -1)\nconst ANY_CASES: List[(Str, Str, Str)] = [\n")
	an := goAnyCases()
	an = append(an, goContainsAnyCases()...)
	ra := rand.New(rand.NewSource(43))
	for i := 0; i < 4000; i++ {
		an = append(an, strsearchRandomAny(ra))
	}
	for _, c := range an {
		c = strsearchText(c)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(c.s), xq(c.sep), xq(strsearchOracleAny(c)))
	}
	o.WriteString("]\n")

	// contains_rune (its own seed)
	rn := goContainsRuneCases()
	rr := rand.New(rand.NewSource(47))
	for i := 0; i < 2000; i++ {
		rn = append(rn, strsearchRandomRune(rr))
	}
	o.WriteString("\n/// (s, rune code point, Go's ContainsRune)\nconst CONTAINS_RUNE_CASES: List[(Str, Int, Bool)] = [\n")
	for _, c := range rn {
		s := textOf(c.s)
		fmt.Fprintf(&o, "  (%s, %d, %v),\n", xq(s), c.r, strings.ContainsRune(s, c.r))
	}
	o.WriteString("]\n")

	// split_n and split_after_n (its own seed)
	sn := goSplitNTests()
	rs := rand.New(rand.NewSource(59))
	for i := 0; i < 3000; i++ {
		sn = append(sn, strsearchRandomSplitN(rs))
	}
	o.WriteString("\n/// (s, sep, n, Go's SplitN pieces, Go's SplitAfterN pieces)\nconst SPLIT_N_CASES: List[(Str, Str, Int, Str, Str)] = [\n")
	for _, c := range sn {
		s := textOf(c.s)
		sep := textOf(c.sep)
		fmt.Fprintf(&o, "  (%s, %s, %d, %s, %s),\n", xq(s), xq(sep), c.n, xq(strsearchList(strings.SplitN(s, sep, c.n))), xq(strsearchList(strings.SplitAfterN(s, sep, c.n))))
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

fn show2(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

fn show_rune(bad: List[(Str, Int, Bool, Bool)]) -> List[(Str, Int, Bool, Bool)] {
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

fn show5(bad: List[(Str, Str, Int, Str, Str)]) -> List[(Str, Str, Int, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "split_n is Go's SplitN" {
  var bad: List[(Str, Str, Int, Str, Str)] = []
  for (s, sep, n, want, _) in SPLIT_N_CASES {
    let got = list_of(s.split_n(sep, n))
    if got != want {
      bad.push((s, sep, n, got, want))
    }
  }
  expect(show5(bad)) == []
}

test "split_after_n is Go's SplitAfterN" {
  var bad: List[(Str, Str, Int, Str, Str)] = []
  for (s, sep, n, _, want) in SPLIT_N_CASES {
    let got = list_of(s.split_after_n(sep, n))
    if got != want {
      bad.push((s, sep, n, got, want))
    }
  }
  expect(show5(bad)) == []
}

test "split_after is Go's SplitAfter, and split_n with n below 0 is split" {
  var bad: List[(Str, Str, Int, Str, Str)] = []
  for (s, sep, n, _, want) in SPLIT_N_CASES {
    if n == -1 {
      let got = list_of(s.split_after(sep))
      if got != want {
        bad.push((s, sep, n, got, want))
      }
    }
    let all = list_of(s.split_n(sep, -1))
    let one = list_of(s.split(sep))
    if all != one {
      bad.push((s, sep, -1, all, one))
    }
  }
  expect(show5(bad)) == []
}

test "split_n, split_after and split_after_n on a view of a longer text" {
  var bad: List[(Str, Str, Int, Str, Str)] = []
  for (s, sep, n, want, after) in SPLIT_N_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = list_of(view.split_n(sep, n))
    if got != want {
      bad.push((s, sep, n, got, want))
    }
    let got_after = list_of(view.split_after_n(sep, n))
    if got_after != after {
      bad.push((s, sep, n, got_after, after))
    }
    if n == -1 {
      let got_plain = list_of(view.split_after(sep))
      if got_plain != after {
        bad.push((s, sep, n, got_plain, after))
      }
    }
  }
  expect(show5(bad)) == []
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

test "fields is Go's Fields" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in FIELDS_CASES {
    let got = list_of(s.fields())
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(show2(bad)) == []
}

test "fields on a view of a longer text" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in FIELDS_CASES {
    let long = "  " + s + "  "
    let view = long[2..(long.len() - 2)]
    let got = list_of(view.fields())
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(show2(bad)) == []
}

test "count is Go's Count" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sub, want) in COUNT_CASES {
    let got = "${s.count(sub)}"
    if got != want {
      bad.push((s, sub, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "count on a view of a longer text" {
  // the slice is exactly s, so every count, the "" one (runes plus one)
  // included, is s's own Go count
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, sub, want) in COUNT_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = "${view.count(sub)}"
    if got != want {
      bad.push((s, sub, got, want))
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

fn any_of(s: Str, chars: Str) -> Str {
  "${s.find_any(chars) ?? -1}\u{1f}${s.rfind_any(chars) ?? -1}\u{1f}${s.contains_any(chars)}"
}

test "find_any, rfind_any and contains_any are Go's IndexAny, LastIndexAny and ContainsAny" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, chars, want) in ANY_CASES {
    let got = any_of(s, chars)
    if got != want {
      bad.push((s, chars, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "find_any, rfind_any and contains_any on a view of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, chars, want) in ANY_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = any_of(view, chars)
    if got != want {
      bad.push((s, chars, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "contains_rune is Go's ContainsRune" {
  var bad: List[(Str, Int, Bool, Bool)] = []
  for (s, cp, want) in CONTAINS_RUNE_CASES {
    let got = s.contains_rune(Rune(cp))
    if got != want {
      bad.push((s, cp, got, want))
    }
  }
  expect(show_rune(bad)) == []
}

test "contains_rune on a view of a longer text" {
  var bad: List[(Str, Int, Bool, Bool)] = []
  for (s, cp, want) in CONTAINS_RUNE_CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = view.contains_rune(Rune(cp))
    if got != want {
      bad.push((s, cp, got, want))
    }
  }
  expect(show_rune(bad)) == []
}
`
