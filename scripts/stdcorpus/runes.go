package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// runeClass is one Rune class method of std/prelude.xo and the Go
// function it must match.
type runeClass struct {
	label  string
	method string
	in     func(rune) bool
}

// runeClasses are the Rune classes of std/prelude.xo in the order the
// corpus prints them. is_print is Go's strconv.IsPrint (decision 0079);
// the rest are Go's unicode package.
var runeClasses = []runeClass{
	{"letters", "is_letter", unicode.IsLetter},
	{"digits", "is_digit", unicode.IsDigit},
	{"spaces", "is_space", unicode.IsSpace},
	{"prints", "is_print", strconv.IsPrint},
	{"upper", "is_upper", unicode.IsUpper},
	{"lower", "is_lower", unicode.IsLower},
	{"title", "is_title", unicode.IsTitle},
	{"punct", "is_punct", unicode.IsPunct},
	{"control", "is_control", unicode.IsControl},
	{"graphic", "is_graphic", unicode.IsGraphic},
	{"mark", "is_mark", unicode.IsMark},
	{"symbol", "is_symbol", unicode.IsSymbol},
	{"number", "is_number", unicode.IsNumber},
}

// runeCaseHash is the hash of the case mappings of the scalar values of a
// plane (Go's ToLower, ToUpper and ToTitle), so every entry of the case
// tables is covered.
func runeCaseHash(plane int) int {
	h := 0
	for i := plane * 0x10000; i < (plane+1)*0x10000; i++ {
		if !utf8.ValidRune(rune(i)) {
			continue
		}
		r := rune(i)
		lo, up, ti := unicode.ToLower(r), unicode.ToUpper(r), unicode.ToTitle(r)
		if int(lo) != i || int(up) != i || int(ti) != i {
			h = (h*31 + i*7 + int(lo)*3 + int(up)*5 + int(ti)*11) % 1000000007
		}
	}
	return h
}

// runePlane is one line of the corpus: the counts of every class in a
// plane of 0x10000 code points, the sum of the UTF-8 lengths, and the
// case hash.
func runePlane(plane int) string {
	counts := make([]int, len(runeClasses))
	lens := 0
	for i := plane * 0x10000; i < (plane+1)*0x10000; i++ {
		if !utf8.ValidRune(rune(i)) {
			continue
		}
		r := rune(i)
		for k, c := range runeClasses {
			if c.in(r) {
				counts[k]++
			}
		}
		lens += utf8.RuneLen(r)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "plane %d:", plane)
	for k, c := range runeClasses {
		fmt.Fprintf(&b, " %s=%d", c.label, counts[k])
	}
	fmt.Fprintf(&b, " len=%d case=%d", lens, runeCaseHash(plane))
	return b.String()
}

// runeLine is one line of the corpus for the code point i: every class,
// the UTF-8 length, and the three case mappings.
func runeLine(i int) string {
	r := rune(i)
	var b strings.Builder
	fmt.Fprintf(&b, "u+%d:", i)
	fmt.Fprintf(&b, " upper=%t lower=%t title=%t punct=%t control=%t graphic=%t mark=%t symbol=%t number=%t len=%d up=%d lo=%d ti=%d",
		unicode.IsUpper(r), unicode.IsLower(r), unicode.IsTitle(r),
		unicode.IsPunct(r), unicode.IsControl(r), unicode.IsGraphic(r),
		unicode.IsMark(r), unicode.IsSymbol(r), unicode.IsNumber(r),
		utf8.RuneLen(r), unicode.ToUpper(r), unicode.ToLower(r), unicode.ToTitle(r))
	return b.String()
}

// runeBounds are the code points whose lines the corpus checks in full:
// the edges of ASCII, of Latin-1, of the two byte UTF-8 range, of the
// surrogate gap, and of the planes.
var runeBounds = []int{0x0, 0x1f, 0x20, 0x41, 0x61, 0x7e, 0x7f, 0x80, 0xa0, 0xff, 0x100, 0x7ff, 0x800, 0xd7ff, 0xe000, 0xffff, 0x10000, 0x10ffff}

// runeNotRunes are the values Rune.try must reject: negative, the
// surrogate range, and above the last code point (core 2.4).
var runeNotRunes = []int{-1, 0xd800, 0xdfff, 0x110000}

func runesCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's unicode package and strconv.IsPrint, on every Unicode scalar value: the counts of each Rune class per plane of 0x10000 code points, the sum of the UTF-8 lengths, and a hash of Go's ToLower, ToUpper and ToTitle; against Rune.is_letter, is_digit, is_space, is_print, is_upper, is_lower, is_title, is_punct, is_control, is_graphic, is_mark, is_symbol, is_number, utf8_len, to_lower, to_upper and to_title, plus the boundaries and the values Rune.try rejects (Go's unicode is Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\n/// One line per plane of 0x10000 code points, plane 0 first, as the test\n")
	o.WriteString("/// below builds them: every class count, the UTF-8 lengths summed, and\n")
	o.WriteString("/// the case hash.\nconst PLANES: List[Str] = [\n")
	for plane := 0; plane <= 16; plane++ {
		fmt.Fprintf(&o, "  %s,\n", xq(runePlane(plane)))
	}
	o.WriteString("]\n\n")

	o.WriteString("/// The bounds of the tables, each in full: (code point, the line).\n")
	o.WriteString("const BOUNDS: List[(Int, Str)] = [\n")
	for _, i := range runeBounds {
		fmt.Fprintf(&o, "  (%d, %s),\n", i, xq(runeLine(i)))
	}
	o.WriteString("]\n\n")

	o.WriteString("/// The values Rune.try must reject: negative, the surrogate range, above\n")
	o.WriteString("/// the last code point.\nconst NOT_RUNES: List[Int] = [")
	for k, i := range runeNotRunes {
		if k > 0 {
			o.WriteString(", ")
		}
		fmt.Fprintf(&o, "%d", i)
	}
	o.WriteString("]\n")

	o.WriteString(runesTests)
	o.WriteString(runeQuoteTable())
	return o.String()
}

// runeQuoteTable is Rune.quote (Go's strconv.QuoteRune) over the printable
// and non printable edges and seeded random scalar values: the audit found
// no call for it (2026-10-26). Only scalar values are rows: Rune(v) faults
// for the surrogate gap, above U+10FFFF and below 0 (core 2.4), so Go's
// U+FFFD form for those is unreachable from a Rune.
func runeQuoteTable() string {
	r := rand.New(rand.NewSource(73))
	vals := []int{0, 1, 6, 7, 8, 9, 10, 11, 12, 13, 26, 27, 28, 31, 32, 33, 34, 39, 92, 126, 127, 128, 0x80, 0x9f, 0xa0, 0xad, 0x200b, 0x2028, 0x2029, 0x3000, 0xfeff, 0xfffd, 0xffff, 0x10000, 0x10ffff, 0xd7ff, 0xe000}
	vals = append(vals, 0x41, 0x61, 0x30, 0xe9, 0x65e5, 0x1f600, 0x1d7ce, 0x4e00)
	for i := 0; i < 400; i++ {
		vals = append(vals, r.Intn(0x110000))
	}
	var o strings.Builder
	o.WriteString("\n/// (the Rune's Int, Go's strconv.QuoteRune)\nconst QUOTES: List[(Int, Str)] = [\n")
	for _, v := range vals {
		if !utf8.ValidRune(rune(v)) {
			continue
		}
		fmt.Fprintf(&o, "  (%d, %s),\n", v, xq(strconv.QuoteRune(rune(v))))
	}
	o.WriteString("]\n")
	o.WriteString(runeQuoteTests)
	return o.String()
}

const runeQuoteTests = `
test "Rune.quote is Go's QuoteRune" {
  expect(QUOTES.len() >= 430)
  var bad: List[Str] = []
  for (v, want) in QUOTES {
    let got = Rune(v).quote()
    if got != want {
      bad.push("u+${v}: ${got} want ${want}")
    }
  }
  expect(show(bad)) == []
}
`

const runesTests = `
fn bit(b: Bool, v: Int) -> Int {
  if b {
    return v
  }
  0
}

fn show(bad: List[Str]) -> List[Str] {
  bad[0..(if bad.len() < 5 { bad.len() } else { 5 })]
}

fn line_of(i: Int) -> Str {
  let r = Rune(i)
  "u+${i}: upper=${r.is_upper()} lower=${r.is_lower()} title=${r.is_title()} punct=${r.is_punct()} control=${r.is_control()} graphic=${r.is_graphic()} mark=${r.is_mark()} symbol=${r.is_symbol()} number=${r.is_number()} len=${r.utf8_len()} up=${Int(r.to_upper())} lo=${Int(r.to_lower())} ti=${Int(r.to_title())}"
}

test "every scalar value is Go's, per plane" {
  var bad: List[Str] = []
  var plane = 0
  while plane <= 16 {
    var letters = 0
    var digits = 0
    var spaces = 0
    var prints = 0
    var uppers = 0
    var lowers = 0
    var titles = 0
    var puncts = 0
    var controls = 0
    var graphics = 0
    var marks = 0
    var symbols = 0
    var numbers = 0
    var lens = 0
    var h = 0
    var i = plane * 0x10000
    while i < (plane + 1) * 0x10000 {
      if let Some(r) = Rune.try(i) {
        letters += bit(r.is_letter(), 1)
        digits += bit(r.is_digit(), 1)
        spaces += bit(r.is_space(), 1)
        prints += bit(r.is_print(), 1)
        uppers += bit(r.is_upper(), 1)
        lowers += bit(r.is_lower(), 1)
        titles += bit(r.is_title(), 1)
        puncts += bit(r.is_punct(), 1)
        controls += bit(r.is_control(), 1)
        graphics += bit(r.is_graphic(), 1)
        marks += bit(r.is_mark(), 1)
        symbols += bit(r.is_symbol(), 1)
        numbers += bit(r.is_number(), 1)
        lens += r.utf8_len()
        let lo = Int(r.to_lower())
        let up = Int(r.to_upper())
        let ti = Int(r.to_title())
        if lo != i || up != i || ti != i {
          h = (h * 31 + i * 7 + lo * 3 + up * 5 + ti * 11) % 1000000007
        }
      }
      i += 1
    }
    let got = "plane ${plane}: letters=${letters} digits=${digits} spaces=${spaces} prints=${prints} upper=${uppers} lower=${lowers} title=${titles} punct=${puncts} control=${controls} graphic=${graphics} mark=${marks} symbol=${symbols} number=${numbers} len=${lens} case=${h}"
    if got != PLANES[plane] {
      bad.push("${got}\n want ${PLANES[plane]}")
    }
    plane += 1
  }
  expect(show(bad)) == []
}

test "the boundaries of the tables" {
  var bad: List[Str] = []
  for (i, want) in BOUNDS {
    let got = line_of(i)
    if got != want {
      bad.push("${got}\n want ${want}")
    }
  }
  expect(show(bad)) == []
}

test "Rune.try rejects what is not a scalar value" {
  var bad: List[Str] = []
  for i in NOT_RUNES {
    match Rune.try(i) {
      Some(r) => bad.push("u+${i} is a rune: ${Int(r)}")
      None => {}
    }
  }
  expect(show(bad)) == []
}
`
