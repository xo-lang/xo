package main

import (
	"fmt"
	"math/rand"
	"strings"
	"unicode"
)

// strsCase is one (s, affix) input of the Str prefix, suffix and trim methods.
type strsCase struct{ s, p string }

// goStringsCases are the inputs of Go's strings_test.go for TrimSpace
// (trimSpaceTests), TrimPrefix and TrimSuffix (trimPrefixTests,
// trimSuffixTests via TrimLeft/TrimRight's affix cases), HasPrefix and
// HasSuffix (indexTests), and Cut/CutPrefix (cutTests). Go's strings tests
// are Copyright 2009 The Go Authors, BSD-style license.
func goStringsCases() []strsCase {
	var cs []strsCase
	for _, s := range []string{
		"", "abc", "abc", "\t\n\v\f\r \u0085 abc                　",
		" ", "  ", "\t", " \t\r\n \t\t\r\r\n\n ", " \t\r\n x\t\t\r\r\n\n ", "  \t\r\n x\t\t\r\r\ny\n 　", "  \t\r\n x\t\t\r\r\ny\n 　x",
		"1 \t\r\n2", " x\x80", "x \xc0", "x \xc0\x80", "x \xc0 ", "x \xc0\xc0 ", "x ☺\xc0\xc0 ", "x ☺ ", "x ☺\xc0 ", "᠎", "᠎a᠎", "\u0085", "a\u0085",
		" x ", " x ", " x ", "\x1f\x1c a \x1d", "\x85", "a\x85", "\xc2", "\xc2\xa0", "\xa0", "\xe2\x80\x80",
	} {
		cs = append(cs, strsCase{s, ""}, strsCase{s, " "}, strsCase{s, "\t"})
	}
	// trimPrefixTests / trimSuffixTests, strings.CutPrefix, CutSuffix
	for _, t := range [][2]string{
		{"", ""}, {"abc", ""}, {"", "abc"}, {"abc", "abc"}, {"abc", "a"}, {"abc", "ab"}, {"abc", "abcd"}, {"abc", "c"}, {"abc", "bc"}, {"abc", "b"},
		{"abc", "d"}, {"aaa", "a"}, {"aaa", "aa"}, {"aaa", "aaa"}, {"aaa", "aaaa"}, {"abab", "ab"}, {"abab", "bab"}, {"abab", "aba"},
		{"foo/bar", "foo/"}, {"foo/bar", "/bar"}, {"foo/bar", "foo/bar"}, {"foo/bar", "/foo"}, {"foo/bar", "bar/"},
		{"日本語", "日"}, {"日本語", "語"}, {"日本語", "日本"}, {"日本語", "本語"}, {"日本語", "\xe6"}, {"日本語", "\x9e"}, {"日本語", "日本語日"},
		{"☺", "☺"}, {"a☺", "☺"}, {"☺a", "☺"}, {"Hello, 世界", "Hello"}, {"Hello, 世界", "世界"}, {"Hello, 世界", "Hello, "}, {"Hello, 世界", "界"},
		{"Bearer abc.def", "Bearer "}, {"Bearer abc.def", "bearer "}, {"Bearer", "Bearer "}, {"Basic dXNlcjpwYXNz", "Basic "},
		{"0123456789abcdef", "0123456789abcdef"}, {"0123456789abcdef", "0123456789abcde"}, {"0123456789abcdef", "123456789abcdef"},
		{"0123456789abcdef0", "0123456789abcdef"}, {"0123456789abcdef0", "123456789abcdef0"}, {"0123456789abcdef0", "0123456789abcdeg"},
		{"0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef"}, {"0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdee"},
		{"0123456789abcdef0123456789abcdef", "123456789abcdef0123456789abcdef"},
	} {
		cs = append(cs, strsCase{t[0], t[1]})
	}
	return cs
}

// strsRandom is a random (s, affix): the affix is a prefix or a suffix of s
// (a quarter of the cases, with one byte changed in a quarter of those) or
// unrelated, and its length covers the eq_at paths (0 to 3, 4 to 16, over 16).
func strsRandom(r *rand.Rand) strsCase {
	alpha := []string{"a", "b", "A", "0", "-", "/", " ", " ", "\t", "\n", "\r", "\v", "\f", "é", "日", "☺", "\u0085", " ", " ", " ", " ", "　", "᠎", "​", "\x1c", "\x1f", "\xc0", "\x80", "\xff"}
	gen := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(alpha[r.Intn(len(alpha))])
		}
		return b.String()
	}
	lens := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 15, 16, 17, 20, 31, 32, 33, 40}
	s := gen(lens[r.Intn(len(lens))])
	if r.Intn(3) == 0 {
		s = strings.Repeat(" ", r.Intn(5)) + s + strings.Repeat(" ", r.Intn(5))
	}
	p := gen(r.Intn(4) + 1)
	switch r.Intn(4) {
	case 0, 1:
		n := lens[r.Intn(len(lens))]
		if n > len(s) {
			n = len(s)
		}
		if r.Intn(2) == 0 {
			p = s[:n]
		} else {
			p = s[len(s)-n:]
		}
		if len(p) > 0 && r.Intn(4) == 0 {
			b := []byte(p)
			b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
			p = string(b)
		}
	}
	return strsCase{s, p}
}

// strsOracle is Go's TrimSpace, TrimLeftFunc and TrimRightFunc of
// unicode.IsSpace, TrimPrefix, TrimSuffix, HasPrefix and HasSuffix, as one
// "\x1f"-separated record.
func strsOracle(c strsCase) string {
	s, p := c.s, c.p
	f := []string{
		strings.TrimSpace(s),
		strings.TrimLeftFunc(s, unicode.IsSpace),
		strings.TrimRightFunc(s, unicode.IsSpace),
		strings.TrimPrefix(s, p),
		strings.TrimSuffix(s, p),
		fmt.Sprint(strings.HasPrefix(s, p)),
		fmt.Sprint(strings.HasSuffix(s, p)),
	}
	return strings.Join(f, "\x1f")
}

func stringsCorpus() string {
	r := rand.New(rand.NewSource(29))
	var o strings.Builder
	o.WriteString(header("strings.TrimSpace, TrimLeftFunc and TrimRightFunc of unicode.IsSpace, TrimPrefix, TrimSuffix, HasPrefix and HasSuffix on the inputs of Go's strings_test.go (trimSpaceTests, trimPrefixTests, trimSuffixTests, indexTests, cutTests) and on seeded random texts, against Str.trim, trim_start, trim_end, trim_prefix, trim_suffix, starts_with and ends_with (Go's strings tests are Copyright 2009 The Go Authors, BSD-style license)"))
	cs := goStringsCases()
	for i := 0; i < 6000; i++ {
		cs = append(cs, strsRandom(r))
	}
	o.WriteString("/// (s, affix, Go's TrimSpace, TrimLeft, TrimRight, TrimPrefix, TrimSuffix, HasPrefix, HasSuffix joined by U+001F)\nconst CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range cs {
		// the inputs are Str: an invalid byte reads as U+FFFD, in the input and the oracle alike
		c.s, c.p = textOf(c.s), textOf(c.p)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(c.s), xq(c.p), xq(strsOracle(c)))
	}
	o.WriteString("]\n")
	o.WriteString(stringsTests)
	return o.String()
}

const stringsTests = `
fn got_of(s: Str, p: Str) -> Str {
  [s.trim(), s.trim_start(), s.trim_end(), s.trim_prefix(p), s.trim_suffix(p), "${s.starts_with(p)}", "${s.ends_with(p)}"].join("\u{1f}")
}

test "trim, trim_start, trim_end, trim_prefix, trim_suffix, starts_with and ends_with are Go's" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, p, want) in CASES {
    let got = got_of(s, p)
    if got != want {
      bad.push((s, p, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "the same methods on a copy and on a slice of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, p, want) in CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = got_of(view, p)
    if got != want {
      bad.push((s, p, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`
