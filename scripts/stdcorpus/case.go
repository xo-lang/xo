package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// caseInputs are the inputs of Str.to_lower and Str.to_upper: the
// boundaries of the 16 byte lane the arm64 backend folds (decision 0373),
// every byte value that sits next to the letter ranges, and seeded random
// mixed case texts.
func caseInputs(r *rand.Rand) []string {
	var cs []string
	// the lane's boundaries: lengths around 8 and 16, so the tail, the
	// words and the runs of the SWAR path all start and end at every
	// offset; the letters alternate case so both directions change bytes
	base := "AbCdEfGhIjKlMnOpQrStUvWxYzAbCdEfGhIjKlMn"
	for _, n := range []int{0, 1, 2, 7, 8, 9, 15, 16, 17, 23, 24, 25, 31, 32, 33} {
		cs = append(cs, base[:n], strings.ToUpper(base[:n]), strings.ToLower(base[:n]))
	}
	// a non-ASCII byte at each position of a 16 and a 17 byte word, both a
	// two byte letter and an invalid byte
	word := "AbCdEfGhIjKlMnOp"
	for i := 0; i <= len(word); i++ {
		for _, c := range []string{"é", "日", "\x80", "\xff", "\u0080"} {
			cs = append(cs, word[:i]+c+word[i:])
		}
	}
	// the bytes next to the letter ranges: @ A Z [ ` a z { and the pairs
	// that straddle a range end
	for _, s := range []string{
		"@[`{", "AZaz", "A[", "Z`", "@A", "[Z", "`a", "{z", "z{", "a`", "Z[", "A@",
		"@AZ[`az{", "Z[@`", "AZaz@[`{", "AaBbZz", "@AaZz[", "`AaZz{",
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz",
		"@ABCDEFGHIJKLMNO", "ABCDEFGHIJKLMNO@", "ZABCDEFGHIJKLMN@",
		"`abcdefghijklmno", "abcdefghijklmno`", "zabcdefghijklmn@{",
		"aAzZ09 /-_", "Hello, World!", "Straße", "İstanbul", "ǅemal",
		"ΑΒΓΔ αβγδ", "КИРИЛЛИЦА кириллица", "日本語 ﾃｷｽﾄ", "Ⅻ ⓐⓑ",
	} {
		cs = append(cs, s)
	}
	// the lane's own boundaries: every byte next to a letter range and the
	// bytes above ASCII, at offsets 0, 15 and 63 inside runs of 16, 17, 64
	// and 65 bytes, where the 16 byte lane and its four lane step are what
	// fold them (a 4 byte run never reaches the lane, which is how the LLVM
	// lane's `ule 26` for `ule 25` shipped: '[' lowered to '{' and '{'
	// raised to '[', and arm64's `cmhs #0x19` was right throughout)
	fill := "AbCdEfGhIjKlMnOpQrStUvWxYzAbCdEfGhIjKlMnOpQrStUvWxYzAbCdEfGhIjKlM"
	for _, n := range []int{16, 17, 64, 65} {
		for _, at := range []int{0, 15, 63} {
			if at >= n {
				continue
			}
			for _, c := range []byte{0x40, 0x5b, 0x60, 0x7b, 0x7f, 0x80, 0xc0, 0xda, 0xff} {
				b := []byte(fill[:n])
				b[at] = c
				cs = append(cs, string(b))
			}
		}
	}
	// seeded random mixed case texts over every alphabet the fold must
	// leave alone as well as the two it changes
	alpha := []string{"a", "b", "z", "A", "B", "Z", "@", "[", "`", "{", "0", "9", " ", "/", "\t", "\n", "é", "É", "日", "\u0080", "߿", "ࠀ", "İ", "ß"}
	for i := 0; i < 2000; i++ {
		var b strings.Builder
		n := r.Intn(40)
		for j := 0; j < n; j++ {
			b.WriteString(alpha[r.Intn(len(alpha))])
		}
		cs = append(cs, b.String())
	}
	return cs
}

// caseCorpus is Str.to_lower and Str.to_upper against Go's
// strings.ToLower and strings.ToUpper, as (input, Go's results joined by
// U+001F).
func caseCorpus() string {
	r := rand.New(rand.NewSource(36))
	var o strings.Builder
	o.WriteString(header("Go's strings.ToLower and strings.ToUpper on the boundaries of the 16 byte case fold (lengths 0 to 33 around 8 and 16, a non-ASCII byte at every position of a word, the bytes next to the letter ranges, and those same bytes at offsets 0, 15 and 63 of runs of 16, 17, 64 and 65 bytes, where the lane folds them) and on seeded random mixed case texts, against Str.to_lower and Str.to_upper"))
	cs := caseInputs(r)
	o.WriteString("/// (s, Go's ToLower and ToUpper joined by U+001F)\nconst CASES: List[(Str, Str)] = [\n")
	seen := map[string]bool{}
	for _, c := range cs {
		// the inputs are Str: an invalid byte reads as U+FFFD, in the
		// input and the oracle alike
		s := textOf(c)
		if seen[s] {
			continue
		}
		seen[s] = true
		want := strings.ToLower(s) + "\u001f" + strings.ToUpper(s)
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(want))
	}
	o.WriteString("]\n")
	o.WriteString(caseTests)
	return o.String()
}

const caseTests = `
fn got_of(s: Str) -> Str {
  "${s.to_lower()}\u{1f}${s.to_upper()}"
}

test "to_lower and to_upper are Go's strings.ToLower and ToUpper" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in CASES {
    let got = got_of(s)
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "the same on a view of a longer text" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in CASES {
    let long = "<<" + s + ">>"
    let view = long[2..(long.len() - 2)]
    let got = got_of(view)
    if got != want {
      bad.push((view, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`
