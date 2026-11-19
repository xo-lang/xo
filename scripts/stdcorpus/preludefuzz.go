package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/hostile"
)

// preludeFuzzCorpus is the prelude's Str and number methods against Go's
// strings and strconv on seeded random texts, separators and numbers, a
// share of them hostile (cut UTF-8, Int's edges as counts, digit runs past
// Int's range): Str.split, split_n, cut, fields, count, find, rfind,
// contains, replace, replace_n, trim_chars, to_upper, to_lower,
// equal_fold; Int.parse, parse_base, to_str_base and Display; Float.parse,
// to_fixed, to_exp, to_general and Display. preludeFuzzSeed is
// XO_PRELUDEFUZZ_SEED (default 0, the committed corpus), added to the
// generator's seeds: internal/stdtest/corpusfuzz_test.go's TestPreludeFuzz
// runs fresh seeds (task #389).
func preludeFuzzSeed() int64 {
	seed, _ := strconv.ParseInt(os.Getenv("XO_PRELUDEFUZZ_SEED"), 10, 64)
	return seed
}

var pfAlphabet = []string{"a", "b", "ab", " ", "\t", "\n", "-", ",", "é", "日", "☺", "ß", "İ", "ﬀ", "K", "Σ", "ς", " ", "\u0085", " ", "　", "x", "A", "Z"}

func pfText(r *rand.Rand, max int) string {
	var b strings.Builder
	n := r.Intn(max + 1)
	for i := 0; i < n; i++ {
		b.WriteString(pfAlphabet[r.Intn(len(pfAlphabet))])
	}
	return b.String()
}

// pfCut is a text with a hostile cut of UTF-8 in it, as the Str the test
// reads (textOf: U+FFFD for each invalid byte)
func pfCut(r *rand.Rand) string {
	cuts := hostile.CutUTF8("日本☺é")
	return textOf(pfText(r, 3) + hostile.Pick(r, cuts) + hostile.Pick(r, hostile.UTF8()) + pfText(r, 3))
}

func pfCount(r *rand.Rand, i int) int64 {
	if i%7 == 3 {
		return hostile.Ints()[(i/7)%len(hostile.Ints())]
	}
	return int64(r.Intn(7) - 2)
}

func pfStrOracle(s, a, b string, n int64) string {
	opt := func(i int) string {
		if i < 0 {
			return "none"
		}
		return strconv.Itoa(i)
	}
	cut := "none"
	if x, y, ok := strings.Cut(s, a); ok {
		cut = x + "\x1e" + y
	}
	nn := int(n)
	return strings.Join([]string{
		strings.Join(strings.Split(s, a), "\x1e"),
		strings.Join(strings.SplitN(s, a, nn), "\x1e"),
		cut,
		strings.Join(strings.Fields(s), "\x1e"),
		strconv.Itoa(strings.Count(s, a)),
		opt(strings.Index(s, a)),
		opt(strings.LastIndex(s, a)),
		fmt.Sprint(strings.Contains(s, a)),
		strings.ReplaceAll(s, a, b),
		strings.Replace(s, a, b, nn),
		strings.Trim(s, a),
		strings.ToUpper(s),
		strings.ToLower(s),
		fmt.Sprint(strings.EqualFold(s, a)),
	}, "\x1f")
}

func pfNumText(r *rand.Rand, i int) string {
	switch i % 6 {
	case 0:
		return hostile.Pick(r, hostile.Digits())
	case 1:
		return hostile.Pick(r, []string{"", "-", "+"}) + hostile.Pick(r, hostile.Hex())
	case 2:
		return strconv.FormatInt(hostile.Pick(r, hostile.Ints()), 10+r.Intn(2)*6)
	}
	pieces := []string{"0", "1", "7", "9", "_", "-", "+", "x", "0x", "0b", "0o", ".", "e", "E", "p", "z", "Z", "inf", "NaN", "Infinity", " ", "f", "a"}
	var b strings.Builder
	for k := r.Intn(8); k >= 0; k-- {
		b.WriteString(pieces[r.Intn(len(pieces))])
	}
	return b.String()
}

func pfFloatText(f float64, fmtc byte, prec int) string {
	return strconv.FormatFloat(f, fmtc, prec, 64)
}

func pfDisplay(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eIN") {
		s += ".0"
	}
	return s
}

func pfNumOracle(text string, base int) string {
	parse := func(base int) string {
		v, err := strconv.ParseInt(text, base, 64)
		if err != nil {
			return "none"
		}
		return strconv.FormatInt(v, 10)
	}
	fl := "none"
	if v, err := strconv.ParseFloat(text, 64); err == nil {
		if math.IsNaN(v) {
			fl = "nan"
		} else {
			fl = strconv.FormatInt(int64(math.Float64bits(v)), 10)
		}
	}
	return strings.Join([]string{parse(10), parse(base), fl}, "\x1f")
}

func preludeFuzzCorpus() string {
	seed := preludeFuzzSeed()
	r := rand.New(rand.NewSource(389 + seed))
	var o strings.Builder
	o.WriteString(header("strings.Split, SplitN, Cut, Fields, Count, Index, LastIndex, Contains, ReplaceAll, Replace, Trim, ToUpper, ToLower and EqualFold, and strconv.ParseInt, FormatInt, ParseFloat and FormatFloat on seeded random and hostile texts and numbers, against the prelude's Str, Int and Float methods"))
	o.WriteString("/// (s, a, b, n as text, Go's Split, SplitN, Cut, Fields, Count, Index, LastIndex, Contains, ReplaceAll, Replace, Trim, ToUpper, ToLower, EqualFold joined by U+001F, lists by U+001E)\nconst STRS: List[(Str, Str, Str, Str, Str)] = [\n")
	for i := 0; i < 2500; i++ {
		var s, a string
		switch i % 5 {
		case 0:
			s = pfCut(r)
			a = textOf(hostile.Pick(r, hostile.UTF8()))
		case 1:
			s = pfText(r, 12)
			a = ""
		default:
			s = pfText(r, 12)
			a = pfText(r, 2)
		}
		b := pfText(r, 2)
		n := pfCount(r, i)
		if !utf8.ValidString(s) || !utf8.ValidString(a) {
			panic("preludefuzz: a text is not UTF-8")
		}
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(s), xq(a), xq(b), xq(strconv.FormatInt(n, 10)), xq(pfStrOracle(s, a, b, n)))
	}
	o.WriteString("]\n\n/// (text, base, Go's ParseInt(text, 10, 64), ParseInt(text, base, 64), ParseFloat's bits or nan, none for an error)\nconst NUMS: List[(Str, Int, Str)] = [\n")
	for i := 0; i < 1500; i++ {
		text := pfNumText(r, i)
		base := []int{0, 2, 8, 10, 16, 36}[r.Intn(6)]
		if r.Intn(3) == 0 {
			base = 2 + r.Intn(35)
		}
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xq(text), base, xq(pfNumOracle(text, base)))
	}
	o.WriteString("]\n\n/// (n as text, base, Go's FormatInt(n, base) and FormatInt(n, 10))\nconst INTS: List[(Str, Int, Str)] = [\n")
	for i := 0; i < 600; i++ {
		var n int64
		if i%2 == 0 {
			n = hostile.Ints()[(i/2)%len(hostile.Ints())]
			if i%4 == 2 {
				n += int64(r.Intn(5)) - 2
			}
		} else {
			n = r.Int63() >> uint(r.Intn(63))
			if r.Intn(2) == 0 {
				n = -n
			}
		}
		base := 2 + r.Intn(35)
		fmt.Fprintf(&o, "  (%s, %d, %s),\n", xq(strconv.FormatInt(n, 10)), base, xq(strconv.FormatInt(n, base)+"\x1f"+strconv.FormatInt(n, 10)))
	}
	o.WriteString("]\n\n/// (bits, prec, Go's FormatFloat 'f', 'e', 'g' at prec, and the Display text)\nconst FLOATS: List[(Int, Int, Str)] = [\n")
	edges := []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 0.5, 1.5, 2.5, 1e21, 1e-7, 123456789.125, math.MaxFloat64, math.SmallestNonzeroFloat64, 2.2250738585072014e-308, math.Inf(1), math.Inf(-1), 9007199254740993, 0.000001, 5e-324, 1.7976931348623157e308}
	for i := 0; i < 1500; i++ {
		var f float64
		switch {
		case i < len(edges):
			f = edges[i]
		case i%3 == 0:
			f = math.Float64frombits(r.Uint64())
		default:
			f = (r.Float64() - 0.5) * math.Pow(10, float64(r.Intn(40)-20))
		}
		if math.IsNaN(f) {
			f = 1
		}
		prec := r.Intn(22) - 1
		want := strings.Join([]string{pfFloatText(f, 'f', prec), pfFloatText(f, 'e', prec), pfFloatText(f, 'g', prec), pfDisplay(f)}, "\x1f")
		fmt.Fprintf(&o, "  (%d, %d, %s),\n", int64(math.Float64bits(f)), prec, xq(want))
	}
	o.WriteString("]\n")
	o.WriteString(preludeFuzzTests)
	return o.String()
}

const preludeFuzzTests = `
fn opt(x: Int?) -> Str {
  match x {
    Some(i) => "${i}"
    None => "none"
  }
}

fn str_got(s: Str, a: Str, b: Str, n: Int) -> Str {
  let cut = match s.cut(a) {
    Some((x, y)) => x + "\u{1e}" + y
    None => "none"
  }
  [
    s.split(a).join("\u{1e}"), s.split_n(a, n).join("\u{1e}"), cut, s.fields().join("\u{1e}"),
    "${s.count(a)}", opt(s.find(a)), opt(s.rfind(a)), "${s.contains(a)}",
    s.replace(a, b), s.replace_n(a, b, n), s.trim_chars(a), s.to_upper(), s.to_lower(),
    "${s.equal_fold(a)}",
  ].join("\u{1f}")
}

fn first(bad: List[Str]) -> List[Str] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "Str's split, split_n, cut, fields, count, find, rfind, contains, replace, replace_n, trim_chars, to_upper, to_lower and equal_fold are Go's" {
  var bad: List[Str] = []
  for (s, a, b, n, want) in STRS {
    let got = str_got(s, a, b, Int.parse(n) ?? 0)
    if got != want {
      bad.push("${s.debug()} ${a.debug()} ${b.debug()} ${n}: got ${got.debug()} want ${want.debug()}")
    }
  }
  expect(first(bad)) == []
}

test "the same Str methods on a slice of a longer text" {
  var bad: List[Str] = []
  for (s, a, b, n, want) in STRS {
    let long = "<<" + s + ">>"
    let got = str_got(long[2..(long.len() - 2)], a, b, Int.parse(n) ?? 0)
    if got != want {
      bad.push("${s.debug()} ${a.debug()} ${n}: got ${got.debug()}")
    }
  }
  expect(first(bad)) == []
}

test "Int.parse, parse_base and Float.parse are Go's ParseInt and ParseFloat" {
  var bad: List[Str] = []
  for (text, base, want) in NUMS {
    let f = match Float.parse(text) {
      Some(x) => if x.is_nan() { "nan" } else { "${x.to_bits()}" }
      None => "none"
    }
    let got = [opt(Int.parse(text)), opt(Int.parse_base(text, base)), f].join("\u{1f}")
    if got != want {
      bad.push("${text.debug()} base ${base}: got ${got.debug()} want ${want.debug()}")
    }
  }
  expect(first(bad)) == []
}

test "Int.to_str_base and Display are Go's FormatInt" {
  var bad: List[Str] = []
  for (text, base, want) in INTS {
    let n = Int.parse(text) ?? 0
    let got = n.to_str_base(base) + "\u{1f}" + "${n}"
    if got != want {
      bad.push("${text} base ${base}: got ${got.debug()} want ${want.debug()}")
    }
  }
  expect(first(bad)) == []
}

test "Float.to_fixed, to_exp, to_general and Display are Go's FormatFloat" {
  var bad: List[Str] = []
  for (bits, prec, want) in FLOATS {
    let f = Float.from_bits(bits)
    let got = [f.to_fixed(prec), f.to_exp(prec), f.to_general(prec), "${f}"].join("\u{1f}")
    if got != want {
      bad.push("${bits} prec ${prec}: got ${got.debug()} want ${want.debug()}")
    }
  }
  expect(first(bad)) == []
}
`
