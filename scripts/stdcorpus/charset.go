package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

// charsetCorpus runs charsetoracle, its own module (golang.org/x/text is
// std/charset's reference, not part of Go's standard library).
func charsetCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "charsetoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("charsetoracle: " + err.Error())
	}
	return string(out) + charsetUTF16()
}

// charsetUTF16: the one-rune functions against Go's unicode/utf16
// (RuneLen, EncodeRune, DecodeRune, AppendRune, IsSurrogate;
// TestStdAPICovered, 2026-11-16), on edge and seeded random values.
func charsetUTF16() string {
	r := rand.New(rand.NewSource(869))
	cps := []rune{0, 0x41, 0x7f, 0xe9, 0xd7ff, 0xd800, 0xdbff, 0xdc00, 0xdfff, 0xe000, 0xfffd, 0xfffe, 0xffff, 0x10000, 0x1f600, 0x10fffe, 0x10ffff, 0x110000, -1, -0xd800, 0x7fffffff}
	for i := 0; i < 40; i++ {
		cps = append(cps, rune(r.Intn(0x120000)))
	}
	var o strings.Builder
	o.WriteString("\n/// (code point, utf16.RuneLen, EncodeRune's pair, AppendRune onto [7])\nconst UTF16_RUNES: List[(Int, Int, Int, Int, List[Int])] = [\n")
	for _, c := range cps {
		r1, r2 := utf16.EncodeRune(c)
		app := utf16.AppendRune([]uint16{7}, c)
		var a []string
		for _, u := range app {
			a = append(a, fmt.Sprint(u))
		}
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, [%s]),\n", c, utf16.RuneLen(c), r1, r2, strings.Join(a, ", "))
	}
	o.WriteString("]\n\n/// (u1, u2, utf16.DecodeRune, IsSurrogate(u1), IsSurrogate(u2))\nconst UTF16_PAIRS: List[(Int, Int, Int, Bool, Bool)] = [\n")
	units := []rune{0, 0x41, 0xd7ff, 0xd800, 0xdbff, 0xdc00, 0xdfff, 0xe000, 0xffff}
	var pairs [][2]rune
	for _, a := range units {
		for _, b := range units {
			pairs = append(pairs, [2]rune{a, b})
		}
	}
	for i := 0; i < 40; i++ {
		pairs = append(pairs, [2]rune{rune(0xd700 + r.Intn(0x1000)), rune(0xd700 + r.Intn(0x1000))})
	}
	for _, p := range pairs {
		fmt.Fprintf(&o, "  (%d, %d, %d, %t, %t),\n", p[0], p[1], utf16.DecodeRune(p[0], p[1]), utf16.IsSurrogate(p[0]), utf16.IsSurrogate(p[1]))
	}
	o.WriteString(`]

test "utf16_rune_len, utf16_encode_rune, utf16_append_rune, utf16_decode_rune and utf16_is_surrogate, as Go's unicode/utf16" {
  for (cp, n, r1, r2, app) in UTF16_RUNES {
    expect(charset.utf16_rune_len(cp)) == n
    expect(charset.utf16_encode_rune(cp)) == (r1, r2)
    expect(charset.utf16_append_rune([7], cp)) == app
  }
  for (u1, u2, want, s1, s2) in UTF16_PAIRS {
    expect(Int(charset.utf16_decode_rune(u1, u2))) == want
    expect(charset.utf16_is_surrogate(u1)) == s1
    expect(charset.utf16_is_surrogate(u2)) == s2
  }
}
`)
	return o.String()
}
