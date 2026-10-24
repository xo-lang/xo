package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// The API this corpus covers, as std.md 2.1 declares it: Int.parse (Go's
// strconv.ParseInt(s, 10, 64)), Int.parse_base (ParseInt(s, base, 64)),
// Int.to_str_base (FormatInt), Bool.parse (ParseBool), Float.parse
// (ParseFloat(s, 64)), the Display of an Int (FormatInt(n, 10)) and the
// Display of a Float (the shortest text that reads back, FormatFloat(f, 'g',
// -1, 64), with ".0" added to a whole number), with Float.from_bits and
// to_bits around it. None stands where Go returns an error, out of range
// included, and never a fault.
//
// The inputs are Go's: number_test.go's parseInt64Tests, parseInt64BaseTests,
// atoftests, atobtests, itob64tests and varlenUints (Go 1.26 has no separate
// atoi, atof, itoa and ftoa test files; this one file holds their tables and
// no TestFtoa table remains, so the shortest-format side of the corpus takes
// its expectations from FormatFloat over seeded bit patterns instead), plus
// seeded integers, bases and float bit patterns. Go's strconv tests are
// Copyright 2009 The Go Authors, BSD-style license.

// strconvTable returns the elements of the composite literal a table var in
// Go's strconv number_test.go holds.
func strconvTable(name string) []*ast.CompositeLit {
	path := filepath.Join(runtime.GOROOT(), "src", "strconv", "number_test.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Go's strconv tests:", err)
		os.Exit(1)
	}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, s := range g.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Names[0].Name != name || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			var out []*ast.CompositeLit
			for _, e := range lit.Elts {
				if c, ok := e.(*ast.CompositeLit); ok {
					out = append(out, c)
				}
			}
			return out
		}
	}
	fmt.Fprintf(os.Stderr, "stdcorpus: Go's number_test.go has no table %s\n", name)
	os.Exit(1)
	return nil
}

// strconvString is a string cell of one of Go's tables.
func strconvString(e ast.Expr) (string, bool) {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		s, err := strconv.Unquote(lit.Value)
		return s, err == nil
	}
	return "", false
}

// strconvInt evaluates the integer expressions Go's tables write: literals
// (an underscore allowed), a parenthesized expression, a negated one, and +,
// -, * and shifts of them.
func strconvInt(e ast.Expr) (int64, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.INT {
			return 0, false
		}
		n, err := strconv.ParseInt(strings.ReplaceAll(v.Value, "_", ""), 0, 64)
		return n, err == nil
	case *ast.ParenExpr:
		return strconvInt(v.X)
	case *ast.UnaryExpr:
		if v.Op == token.SUB {
			n, ok := strconvInt(v.X)
			return -n, ok
		}
	case *ast.BinaryExpr:
		a, ok1 := strconvInt(v.X)
		b, ok2 := strconvInt(v.Y)
		if !ok1 || !ok2 || b < 0 || b > 63 {
			return 0, false
		}
		switch v.Op {
		case token.ADD:
			return a + b, true
		case token.SUB:
			return a - b, true
		case token.MUL:
			return a * b, true
		case token.SHL:
			return a << uint(b), true
		case token.SHR:
			return a >> uint(b), true
		}
	}
	return 0, false
}

// strconvRandomDigits is a run of decimal digits of length n, no zero first
// digit unless n is 1.
func strconvRandomDigits(r *rand.Rand, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		d := r.Intn(10)
		if i == 0 && n > 1 && d == 0 {
			d = 1
		}
		b.WriteByte(byte('0' + d))
	}
	return b.String()
}

func strconvCorpus() string {
	var o strings.Builder
	o.WriteString(header("strconv.ParseInt, ParseInt with a base, FormatInt, ParseBool, ParseFloat and FormatFloat's shortest text on the inputs of Go's strconv number_test.go (parseInt64Tests, parseInt64BaseTests, atoftests, atobtests, itob64tests and varlenUints) and on seeded integers, bases and float bit patterns, against Int.parse, Int.parse_base, Int.to_str_base, Bool.parse, Float.parse, the Display of an Int and of a Float, and Float.from_bits and to_bits (Go's strconv tests are Copyright 2009 The Go Authors, BSD-style license)"))

	// Int.parse: Go's ParseInt(s, 10, 64). The inputs are Go's, then seeded
	// decimal texts, then the shapes Go's base-10 parser refuses.
	var ints []string
	for _, c := range strconvTable("parseInt64Tests") {
		if s, ok := strconvString(c.Elts[0]); ok {
			ints = append(ints, s)
		}
	}
	for _, c := range strconvTable("parseInt64BaseTests") {
		if s, ok := strconvString(c.Elts[0]); ok {
			ints = append(ints, s)
		}
	}
	// The shapes Go's base 10 parser decides on: signs, leading zeros,
	// underscores (base 0 only), surrounding space, the ends of the range.
	ints = append(ints, "0", "-0", "+0", "007", "-007", "9223372036854775807",
		"-9223372036854775808", "9223372036854775808", "-9223372036854775809",
		"18446744073709551615", "9999999999999999999", "1_000", "1_0", "_1",
		" 1", "1 ", "\t1", "+", "-", "0x10", "0b11", "1e3", "12.5", "١٢")
	ri := rand.New(rand.NewSource(83))
	for i := 0; i < 400; i++ {
		s := strconvRandomDigits(ri, 1+ri.Intn(25))
		switch ri.Intn(4) {
		case 0:
			s = "-" + s
		case 1:
			s = "+" + s
		}
		ints = append(ints, s)
	}
	o.WriteString("\n/// (text, whether Go's ParseInt(s, 10, 64) accepts it, its value; zero on an error)\nconst INT_CASES: List[(Str, Bool, Int)] = [\n")
	for _, s := range timeDedup(ints) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			// out of range is a None here too: Go's value is clamped
			fmt.Fprintf(&o, "  (%s, false, 0),\n", xq(s))
			continue
		}
		fmt.Fprintf(&o, "  (%s, true, %d),\n", xq(s), n)
	}
	o.WriteString("]\n")

	// Int.parse_base: Go's ParseInt(s, base, 64) for base 0 and 2 to 36.
	type baseRow struct {
		in   string
		base int
	}
	var bases []baseRow
	for _, c := range strconvTable("parseInt64BaseTests") {
		in, ok1 := strconvString(c.Elts[0])
		b, ok2 := strconvInt(c.Elts[1])
		if ok1 && ok2 {
			bases = append(bases, baseRow{in, int(b)})
		}
	}
	// Seeded: a value written by Go in the base and read back, plus the
	// prefixes and separators base 0 takes.
	rb := rand.New(rand.NewSource(89))
	baseList := []int{0, 2, 3, 8, 9, 10, 16, 26, 32, 36}
	for i := 0; i < 400; i++ {
		v := rb.Int63() - rb.Int63()
		base := baseList[rb.Intn(len(baseList))]
		if base == 0 {
			// FormatInt has no base 0; the decimal text reads the same in
			// base 0, which is the base under test
			base = 10
		}
		text := strconv.FormatInt(v, base)
		if base == 10 && rb.Intn(2) == 0 {
			// half of the decimal rows go through base 0
			base = 0
		}
		bases = append(bases, baseRow{text, base})
	}
	bases = append(bases, baseRow{"0b1011", 0}, baseRow{"0o755", 0},
		baseRow{"0x1f", 0}, baseRow{"1_000", 0}, baseRow{"1_000", 10},
		baseRow{"0b", 0}, baseRow{"0x", 0}, baseRow{"z", 36},
		baseRow{"Z", 36}, baseRow{"-0b1011", 0}, baseRow{"+0x1f", 0})
	o.WriteString("\n/// (text, base, whether Go's ParseInt(s, base, 64) accepts it, its value)\nconst PARSE_BASE_CASES: List[(Str, Int, Bool, Int)] = [\n")
	for _, r := range bases {
		n, err := strconv.ParseInt(r.in, r.base, 64)
		if err != nil {
			fmt.Fprintf(&o, "  (%s, %d, false, 0),\n", xq(r.in), r.base)
			continue
		}
		fmt.Fprintf(&o, "  (%s, %d, true, %d),\n", xq(r.in), r.base, n)
	}
	o.WriteString("]\n")

	// Int.to_str_base: Go's FormatInt.
	type itoaRow struct {
		n    int64
		base int
		out  string
	}
	var itoas []itoaRow
	for _, c := range strconvTable("itob64tests") {
		n, ok1 := strconvInt(c.Elts[0])
		b, ok2 := strconvInt(c.Elts[1])
		if ok1 && ok2 {
			itoas = append(itoas, itoaRow{n, int(b), strconv.FormatInt(n, int(b))})
		}
	}
	for _, c := range strconvTable("varlenUints") {
		// the signed values only: 12345678901234567890 is past Int
		if n, ok := strconvInt(c.Elts[0]); ok {
			itoas = append(itoas, itoaRow{n, 10, strconv.FormatInt(n, 10)})
		}
	}
	rt := rand.New(rand.NewSource(97))
	for i := 0; i < 400; i++ {
		n := rt.Int63() - rt.Int63()
		base := 2 + rt.Intn(35)
		itoas = append(itoas, itoaRow{n, base, strconv.FormatInt(n, base)})
	}
	itoas = append(itoas,
		itoaRow{math.MinInt64, 10, strconv.FormatInt(math.MinInt64, 10)},
		itoaRow{math.MaxInt64, 2, strconv.FormatInt(math.MaxInt64, 2)},
		itoaRow{-1, 36, "-1"}, itoaRow{35, 36, "z"}, itoaRow{0, 2, "0"})
	o.WriteString("\n/// (value, base, Go's FormatInt)\nconst TO_STR_BASE_CASES: List[(Int, Int, Str)] = [\n")
	for _, r := range itoas {
		fmt.Fprintf(&o, "  (%d, %d, %s),\n", r.n, r.base, xq(r.out))
	}
	o.WriteString("]\n")

	// Bool.parse: Go's ParseBool, plus its spellings and the near misses.
	o.WriteString("\n/// (text, whether Go's ParseBool accepts it, its value)\nconst BOOL_CASES: List[(Str, Bool, Bool)] = [\n")
	var bools []string
	for _, c := range strconvTable("atobtests") {
		if s, ok := strconvString(c.Elts[0]); ok {
			bools = append(bools, s)
		}
	}
	bools = append(bools, "1", "t", "T", "TRUE", "true", "True", "0", "f", "F",
		"FALSE", "false", "False", "TRue", "tRue", "true ", " true", "yes",
		"2", "FalsE", "1.0")
	for _, s := range timeDedup(bools) {
		b, err := strconv.ParseBool(s)
		if err != nil {
			fmt.Fprintf(&o, "  (%s, false, false),\n", xq(s))
			continue
		}
		fmt.Fprintf(&o, "  (%s, true, %v),\n", xq(s), b)
	}
	o.WriteString("]\n")

	// Float.parse: Go's ParseFloat(s, 64), then the Display text, which is
	// the shortest text that reads back with ".0" on a whole number.
	var floats []string
	for _, c := range strconvTable("atoftests") {
		if s, ok := strconvString(c.Elts[0]); ok {
			floats = append(floats, s)
		}
	}
	floats = append(floats, "0", "-0", "1", "1.0", "0.1", "1e21", "1e-7",
		"5e-324", "1.7976931348623157e+308", "-1.7976931348623157e+308",
		"2.2250738585072014e-308", "1_000.5", "1_0", "0x1p-2", "0x1.8p1",
		"inf", "-Inf", "nan", "NaN", "+inf", "infinity", "-Infinity",
		"1e400", "-1e400", "1e-400", "0.", ".5", ".", "1e", "e1", "1.2.3",
		"  1.5", "1.5  ", "+1.5e+03", "-0.0")
	rf := rand.New(rand.NewSource(101))
	for i := 0; i < 400; i++ {
		var f float64
		switch i % 4 {
		case 0:
			f = math.Float64frombits(rf.Uint64())
		case 1:
			f = rf.NormFloat64()
		case 2:
			f = float64(rf.Int63()) / float64(int64(1)<<uint(1+rf.Intn(40)))
		default:
			f = rf.NormFloat64() * math.Pow(10, float64(rf.Intn(600)-300))
		}
		floats = append(floats, strconv.FormatFloat(f, 'g', -1, 64))
	}
	o.WriteString("\n/// (text, whether Go's ParseFloat(s, 64) accepts it, its Display text; empty on an error)\nconst FLOAT_CASES: List[(Str, Bool, Str)] = [\n")
	for _, s := range timeDedup(floats) {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			// ErrRange returns the clamped +-Inf and an error: a None to Xo
			fmt.Fprintf(&o, "  (%s, false, \"\"),\n", xq(s))
			continue
		}
		fmt.Fprintf(&o, "  (%s, true, %s),\n", xq(s), xq(xoFloat(f)))
	}
	o.WriteString("]\n")

	// The Display of a Float straight from its bits, which is how the
	// cases with no literal spelling (NaN, the subnormals, the smallest
	// and largest values) get in, and the bits back.
	var bits []uint64
	for _, f := range []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 1e21,
		1e-7, math.MaxFloat64, math.SmallestNonzeroFloat64,
		math.Float64frombits(0x001fffffffffffff), math.Inf(1), math.Inf(-1),
		math.NaN(), 3.141592653589793, 1.5, 2.5, 0.5, 1e6, 1e20, 1e-6} {
		bits = append(bits, math.Float64bits(f))
	}
	rfb := rand.New(rand.NewSource(103))
	for i := 0; i < 400; i++ {
		bits = append(bits, rfb.Uint64())
	}
	o.WriteString("\n/// (IEEE 754 bits, the Display text of the Float they hold)\nconst FLOAT_BITS_CASES: List[(Int, Str)] = [\n")
	for _, b := range bits {
		fmt.Fprintf(&o, "  (%d, %s),\n", int64(b), xq(xoFloat(math.Float64frombits(b))))
	}
	o.WriteString("]\n")

	// The Display of an Int.
	o.WriteString("\n/// (value, Go's FormatInt(value, 10))\nconst INT_DISPLAY_CASES: List[(Int, Str)] = [\n")
	vals := []int64{0, 1, -1, 9, 10, -10, 99, 100, math.MaxInt64, math.MinInt64,
		1000000007, -1234567890123}
	rv := rand.New(rand.NewSource(107))
	for i := 0; i < 200; i++ {
		vals = append(vals, rv.Int63()-rv.Int63())
	}
	for _, v := range vals {
		fmt.Fprintf(&o, "  (%d, %s),\n", v, xq(strconv.FormatInt(v, 10)))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "Int.parse is Go's ParseInt(s, 10, 64), None where Go errors" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want) in INT_CASES {
    let got = match Int.parse(text) {
      Some(n) => "${n}"
      None => "!none"
    }
    let want_text = if ok { "${want}" } else { "!none" }
    if got != want_text {
      bad.push((text, got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "Int.parse_base is Go's ParseInt(s, base, 64)" {
  var bad: List[(Str, Str, Str)] = []
  for (text, base, ok, want) in PARSE_BASE_CASES {
    let got = match Int.parse_base(text, base) {
      Some(n) => "${n}"
      None => "!none"
    }
    let want_text = if ok { "${want}" } else { "!none" }
    if got != want_text {
      bad.push(("${text} base ${base}", got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "to_str_base is Go's FormatInt" {
  var bad: List[(Str, Str, Str)] = []
  for (n, base, want) in TO_STR_BASE_CASES {
    let got = n.to_str_base(base)
    if got != want {
      bad.push(("${n} base ${base}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "Bool.parse is Go's ParseBool, None where Go errors" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want) in BOOL_CASES {
    let got = match Bool.parse(text) {
      Some(b) => "${b}"
      None => "!none"
    }
    let want_text = if ok { "${want}" } else { "!none" }
    if got != want_text {
      bad.push((text, got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "Float.parse is Go's ParseFloat(s, 64), None where Go errors" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want) in FLOAT_CASES {
    let got = match Float.parse(text) {
      Some(f) => "${f}"
      None => "!none"
    }
    let want_text = if ok { want } else { "!none" }
    if got != want_text {
      bad.push((text, got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "a Float displays as the shortest text that reads back, .0 on a whole number" {
  var bad: List[(Str, Str, Str)] = []
  for (bits, want) in FLOAT_BITS_CASES {
    let f = Float.from_bits(bits)
    let got = "${f}"
    if got != want {
      bad.push(("bits ${bits}", got, want))
    }
    let bits_back = f.to_bits()
    if bits_back != bits {
      bad.push(("bits ${bits}", "${bits_back}", "${bits}"))
    }
    // A NaN's display is "NaN" and reads back as a NaN, not as the same
    // bits: no text carries a NaN's payload, in Go either.
    if want == "NaN" {
      let back = match Float.parse(got) {
        Some(g) => g.is_nan()
        None => false
      }
      if !back {
        bad.push(("reads back ${bits}", "not a NaN", "a NaN"))
      }
    } else {
      let back = match Float.parse(got) {
        Some(g) => if g.to_bits() == bits { "same" } else { "other" }
        None => "!none"
      }
      if back != "same" {
        bad.push(("reads back ${bits}", back, "same"))
      }
    }
  }
  expect(show3(bad)) == []
}

test "an Int displays as Go's FormatInt(n, 10)" {
  var bad: List[(Str, Str, Str)] = []
  for (n, want) in INT_DISPLAY_CASES {
    let got = "${n}"
    if got != want {
      bad.push(("${n}", got, want))
    }
  }
  expect(show3(bad)) == []
}
`)
	return o.String()
}
