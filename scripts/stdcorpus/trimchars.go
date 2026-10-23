package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// trimCase is one (s, chars) input of Str.trim_chars, Str.trim_start_chars
// and Str.trim_end_chars; the results come from Go's strings.Trim,
// TrimLeft and TrimRight. Go's strings tests (trimTests) are Copyright
// 2009 The Go Authors, BSD-style license.
type trimCase struct{ s, cutset string }

// goTrimCases are Go's trimTests rows for Trim, TrimLeft and TrimRight,
// deduplicated (one row serves all three methods), checked against Go's
// own expected output as it is read.
func goTrimCases() []trimCase {
	var cs []trimCase
	seen := map[trimCase]bool{}
	for _, t := range []struct {
		f            string
		in, arg, out string
	}{
		{"Trim", "abba", "a", "bb"},
		{"Trim", "abba", "ab", ""},
		{"TrimLeft", "abba", "ab", ""},
		{"TrimRight", "abba", "ab", ""},
		{"TrimLeft", "abba", "a", "bba"},
		{"TrimLeft", "abba", "b", "abba"},
		{"TrimRight", "abba", "a", "abb"},
		{"TrimRight", "abba", "b", "abba"},
		{"Trim", "<tag>", "<>", "tag"},
		{"Trim", "* listitem", " *", "listitem"},
		{"Trim", `"quote"`, `"`, "quote"},
		{"Trim", "ⱯⱯɐɐⱯⱯ", "Ɐ", "ɐɐ"},
		{"Trim", "\x80test\xff", "\xff", "test"},
		{"Trim", " Ġ ", " ", "Ġ"},
		{"Trim", " Ġİ0", "0 ", "Ġİ"},
		{"Trim", "abba", "", "abba"},
		{"Trim", "", "123", ""},
		{"Trim", "", "", ""},
		{"TrimLeft", "abba", "", "abba"},
		{"TrimLeft", "", "123", ""},
		{"TrimLeft", "", "", ""},
		{"TrimRight", "abba", "", "abba"},
		{"TrimRight", "", "123", ""},
		{"TrimRight", "", "", ""},
		{"TrimRight", "☺\xc0", "☺", "☺\xc0"},
	} {
		in, arg := textOf(t.in), textOf(t.arg)
		var got string
		switch t.f {
		case "Trim":
			got = strings.Trim(in, arg)
		case "TrimLeft":
			got = strings.TrimLeft(in, arg)
		case "TrimRight":
			got = strings.TrimRight(in, arg)
		default:
			panic("trimchars: unknown trimTests function " + t.f)
		}
		if want := textOf(t.out); got != want {
			panic(fmt.Sprintf("trimchars: Go's %s(%q, %q) = %q, its trimTests row says %q", t.f, in, arg, got, want))
		}
		c := trimCase{in, arg}
		if !seen[c] {
			seen[c] = true
			cs = append(cs, c)
		}
	}
	return cs
}

// trimAlphabet is the random text alphabet: ASCII, two, three and four
// byte runes, and three bytes that are not valid UTF-8 (textOf reads each
// as U+FFFD, decision 0259, so the corpus holds what an Xo Str holds).
var trimAlphabet = []string{"a", "b", "c", "A", "Z", " ", "-", ".", "0", "é", "ß", "☺", "Σ", "語", "😀", "\x80", "\xff", "\xc0"}

var trimLens = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 15, 16, 17, 20, 31, 40}

func trimLen(r *rand.Rand) int { return trimLens[r.Intn(len(trimLens))] }

// trimRandomCutset is a random cutset: empty, one ASCII byte, several
// ASCII bytes, one multi-byte rune, a mix of runes, or one holding an
// invalid byte.
func trimRandomCutset(r *rand.Rand) string {
	var b strings.Builder
	switch r.Intn(6) {
	case 0:
		return ""
	case 1:
		b.WriteString(trimAlphabet[r.Intn(9)])
	case 2:
		for i := 0; i < 2+r.Intn(3); i++ {
			b.WriteString(trimAlphabet[r.Intn(9)])
		}
	default:
		for i := 0; i < 1+r.Intn(4); i++ {
			b.WriteString(trimAlphabet[r.Intn(len(trimAlphabet))])
		}
	}
	return b.String()
}

// trimRandomCase is a random (s, chars): the text may hold the cutset's
// own characters at either end, so that trimming has work to do.
func trimRandomCase(r *rand.Rand) trimCase {
	cutset := trimRandomCutset(r)
	var b strings.Builder
	if cutset != "" && r.Intn(2) == 0 {
		for i := 0; i < 1+r.Intn(3); i++ {
			k := r.Intn(len(cutset))
			b.WriteString(cutset[k : k+1])
		}
	}
	for i := 0; i < trimLen(r); i++ {
		b.WriteString(trimAlphabet[r.Intn(len(trimAlphabet))])
	}
	if cutset != "" && r.Intn(2) == 0 {
		for i := 0; i < 1+r.Intn(3); i++ {
			k := r.Intn(len(cutset))
			b.WriteString(cutset[k : k+1])
		}
	}
	return trimCase{b.String(), cutset}
}

func trimcharsCorpus() string {
	var o strings.Builder
	o.WriteString(header("strings.Trim, TrimLeft and TrimRight on the inputs of Go's strings_test.go (trimTests) and on seeded random texts and cutsets, against Str.trim_chars, Str.trim_start_chars and Str.trim_end_chars (Go's strings tests are Copyright 2009 The Go Authors, BSD-style license)"))
	r := rand.New(rand.NewSource(67))
	cs := goTrimCases()
	for i := 0; i < 3000; i++ {
		cs = append(cs, trimRandomCase(r))
	}
	o.WriteString("\n/// (s, chars, Go's Trim, TrimLeft and TrimRight, joined by U+001F)\nconst TRIM_CASES: List[(Str, Str, Str, Str, Str)] = [\n")
	for _, c := range cs {
		s, chars := textOf(c.s), textOf(c.cutset)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(s), xq(chars),
			xq(strings.Trim(s, chars)), xq(strings.TrimLeft(s, chars)), xq(strings.TrimRight(s, chars)))
	}
	o.WriteString("]\n")
	o.WriteString(trimcharsTests)
	return o.String()
}

const trimcharsTests = `
fn show4(bad: List[(Str, Str, Str, Str, Str, Str)]) -> List[(Str, Str, Str, Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

fn trim_of(s: Str, chars: Str) -> Str {
  "${s.trim_chars(chars)}\u{1f}${s.trim_start_chars(chars)}\u{1f}${s.trim_end_chars(chars)}"
}

test "trim_chars, trim_start_chars and trim_end_chars are Go's Trim, TrimLeft and TrimRight" {
  var bad: List[(Str, Str, Str, Str, Str, Str)] = []
  for (s, chars, trim, left, right) in TRIM_CASES {
    let got = trim_of(s, chars)
    let want = "${trim}\u{1f}${left}\u{1f}${right}"
    if got != want {
      bad.push((s, chars, got, want, trim, left))
    }
  }
  expect(show4(bad)) == []
}

test "trim_chars on a view of a longer text" {
  var bad: List[(Str, Str, Str, Str, Str, Str)] = []
  for (s, chars, trim, left, right) in TRIM_CASES {
    let long = "##" + s + "##"
    let view = long[2..(long.len() - 2)]
    let got = trim_of(view, chars)
    let want = "${trim}\u{1f}${left}\u{1f}${right}"
    if got != want {
      bad.push((s, chars, got, want, trim, left))
    }
  }
  expect(show4(bad)) == []
}
`
