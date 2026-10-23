package main

import (
	"fmt"
	"math/rand"
	"strings"
	"unicode"
)

// equalfoldCase is one (s, t) input of Str.equal_fold.
type equalfoldCase struct{ s, t string }

// goEqualFoldCases are Go's strings_test.go EqualFoldTests. Go's strings
// tests are Copyright 2009 The Go Authors, BSD-style license. The table's
// own expected values are checked against strings.EqualFold while the
// corpus is generated, so a Go change that contradicted its test would
// stop the generator rather than record either answer.
func goEqualFoldCases() []equalfoldCase {
	var cs []equalfoldCase
	for _, tt := range []struct {
		s, t string
		out  bool
	}{
		{"abc", "abc", true},
		{"ABcd", "ABcd", true},
		{"123abc", "123ABC", true},
		{"αβδ", "ΑΒΔ", true},
		{"abc", "xyz", false},
		{"abc", "XYZ", false},
		{"abcdefghijk", "abcdefghijX", false},
		{"abcdefghijk", "abcdefghijK", true},
		{"abcdefghijK", "abcdefghijK", true},
		{"abcdefghijkz", "abcdefghijKy", false},
		{"abcdefghijKz", "abcdefghijKy", false},
		{"1", "2", false},
		{"utf-8", "US-ASCII", false},
	} {
		if got := strings.EqualFold(tt.s, tt.t); got != tt.out {
			panic(fmt.Sprintf("Go disagrees with its EqualFoldTests: EqualFold(%q, %q) = %v, table says %v", tt.s, tt.t, got, tt.out))
		}
		// Go's TestEqualFold checks both directions.
		cs = append(cs, equalfoldCase{tt.s, tt.t}, equalfoldCase{tt.t, tt.s})
	}
	return cs
}

// equalfoldOrbitSeeds are one rune of each simple case folding orbit the
// random cases draw from: the ASCII pairs, the strings Go's own tests use
// (k, K and the Kelvin sign; s, S and the long s), the Greek orbits, the
// ring and ohm signs, the sharp s, the two Turkish I's, whose orbits hold
// only themselves under simple folding (dotted and dotless I do not fold
// to i and I), and two Cyrillic orbits. The orbits themselves are walked
// from unicode.SimpleFold below, so their members are Go's data.
var equalfoldOrbitSeeds = []rune{
	'k', 's', 'a', 'z', '0', ' ', '-',
	'σ', 'β', 'ε', 'θ', 'ι', 'κ', 'μ',
	'π', 'ρ', 'φ', 'ω',
	'å', 'ß', 'İ', 'ı',
	'в', 'д',
}

// equalfoldOrbits is every orbit of equalfoldOrbitSeeds, in walk order.
var equalfoldOrbits = func() [][]rune {
	var out [][]rune
	for _, r := range equalfoldOrbitSeeds {
		o := []rune{r}
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			o = append(o, f)
		}
		out = append(out, o)
	}
	return out
}()

// equalfoldRandom is a random (s, t) pair drawn orbit by orbit: each rune
// of s and of t comes from the same orbit, so the pair is fold-equal; one
// in four pairs has one rune of t replaced from a different orbit, and
// one in eight drops a rune or appends a tail to t, so it is not.
func equalfoldRandom(r *rand.Rand) equalfoldCase {
	var a, b strings.Builder
	n := r.Intn(7)
	for i := 0; i < n; i++ {
		o := equalfoldOrbits[r.Intn(len(equalfoldOrbits))]
		a.WriteRune(o[r.Intn(len(o))])
		b.WriteRune(o[r.Intn(len(o))])
	}
	switch k := r.Intn(8); {
	case k == 0 && n > 0:
		// a different orbit at one position
		o := equalfoldOrbits[r.Intn(len(equalfoldOrbits))]
		rs := []rune(b.String())
		rs[r.Intn(len(rs))] = o[r.Intn(len(o))]
		return equalfoldCase{a.String(), string(rs)}
	case k == 1 && n > 0:
		// one more rune on t
		o := equalfoldOrbits[r.Intn(len(equalfoldOrbits))]
		return equalfoldCase{a.String(), b.String() + string(o[r.Intn(len(o))])}
	case k == 2 && n > 1:
		// one rune fewer on t
		rs := []rune(b.String())
		i := r.Intn(len(rs))
		return equalfoldCase{a.String(), string(append(rs[:i:i], rs[i+1:]...))}
	}
	return equalfoldCase{a.String(), b.String()}
}

func equalfoldCorpus() string {
	var o strings.Builder
	o.WriteString(header("strings.EqualFold on the inputs of Go's strings_test.go (EqualFoldTests) and on seeded random pairs drawn from the simple case folding orbits of unicode.SimpleFold (k, K and the Kelvin sign; s, S and the long s; the Greek sigma, beta, epsilon, theta, iota, kappa, mu, pi, rho, phi and omega orbits; the ring and ohm signs; the sharp s; the Turkish dotted and dotless I, whose orbits hold only themselves; two Cyrillic orbits), against Str.equal_fold (Go's strings tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\n/// (s, t, Go's EqualFold)\nconst EQUAL_FOLD_CASES: List[(Str, Str, Str)] = [\n")
	cases := goEqualFoldCases()
	r := rand.New(rand.NewSource(43))
	for i := 0; i < 3000; i++ {
		cases = append(cases, equalfoldRandom(r))
	}
	for _, c := range cases {
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(textOf(c.s)), xq(textOf(c.t)), xq(fmt.Sprintf("%v", strings.EqualFold(c.s, c.t))))
	}
	o.WriteString("]\n")
	o.WriteString(equalfoldTests)
	return o.String()
}

const equalfoldTests = `
fn show3(bad: List[(Str, Str, Str, Str)]) -> List[(Str, Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "equal_fold is Go's EqualFold" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, t, want) in EQUAL_FOLD_CASES {
    let got = "${s.equal_fold(t)}"
    if got != want {
      bad.push((s, t, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "equal_fold on views of a longer text" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (s, t, want) in EQUAL_FOLD_CASES {
    let ls = "<<" + s + ">>"
    let lt = "[" + t + "]"
    let vs = ls[2..(ls.len() - 2)]
    let vt = lt[1..(lt.len() - 1)]
    let got = "${vs.equal_fold(vt)}"
    if got != want {
      bad.push((s, t, got, want))
    }
  }
  expect(show3(bad)) == []
}
`
