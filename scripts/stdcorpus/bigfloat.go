package main

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"strings"
)

// bfPrecs are the precisions operands and results take: math/big's word
// boundaries, float32 and float64, and a few wide ones.
var bfPrecs = []uint{1, 2, 3, 5, 10, 24, 53, 63, 64, 65, 100, 127, 128, 129, 200, 300, 600}

// bfVal is a random operand: zero (both signs), an infinity, or a finite
// value of a random precision and mode with a mantissa of up to that many
// bits (runs of ones and zeros) and an exponent mostly near 0, sometimes
// near math/big's limits.
func bfVal(r *rand.Rand, small bool) (*big.Float, string) {
	prec := bfPrecs[r.Intn(len(bfPrecs))]
	mode := big.RoundingMode(r.Intn(6))
	neg := r.Intn(2) == 0
	sign := "+"
	if neg {
		sign = "-"
	}
	z := new(big.Float).SetPrec(prec).SetMode(mode)
	switch k := r.Intn(30); {
	case k == 0:
		z.SetInt64(0)
		if neg {
			z.Neg(z)
		}
		return z, fmt.Sprintf("Z:%s:0:0:%d:%d", sign, prec, mode)
	case k == 1 && !small:
		z.SetInf(neg)
		return z, fmt.Sprintf("I:%s:0:0:%d:%d", sign, prec, mode)
	}
	bits := 1 + r.Intn(int(prec))
	m := bigRand(r, bits)
	m.Abs(m)
	if m.BitLen() > bits || m.Sign() == 0 {
		m.SetInt64(1)
	}
	for m.Bit(0) == 0 {
		m.Rsh(m, 1)
	}
	exp := r.Intn(161) - 80
	if !small {
		switch r.Intn(12) {
		case 0:
			exp = math.MaxInt32 - r.Intn(3)
		case 1:
			exp = math.MinInt32 + r.Intn(3)
		case 2:
			exp = r.Intn(2200) - 1100
		}
	}
	z.SetInt(m)
	z.SetMantExp(z, exp-m.BitLen())
	if neg {
		z.Neg(z)
	}
	z.SetMode(mode)
	return z, fmt.Sprintf("F:%s:%x:%d:%d:%d", sign, m, exp, prec, mode)
}

// bfRep is a result as the corpus compares it: form, sign, precision,
// mode, accuracy, mantissa in hex (odd) and exponent.
func bfRep(z *big.Float) string {
	form, mant, exp := 1, "0", 0
	switch {
	case z.IsInf():
		form = 2
	case z.Sign() == 0:
		form = 0
	default:
		m := new(big.Float)
		exp = z.MantExp(m)
		m.SetMantExp(m, int(z.MinPrec()))
		i, _ := m.Int(nil)
		mant = i.Abs(i).Text(16)
	}
	sign := "+"
	if z.Signbit() {
		sign = "-"
	}
	return fmt.Sprintf("%d %s %d %s %s %s %d", form, sign, z.Prec(), z.Mode(), z.Acc(), mant, exp)
}

// bfTry runs f, reporting whether math/big panicked (ErrNaN): those
// operations fault in Xo and are not rows.
func bfTry(f func() string) (s string, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return f(), true
}

// bfParseInput is a text for ParseFloat: numbers in every base and form,
// and malformed ones.
func bfParseInput(r *rand.Rand) string {
	digits := func(n int, set string) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(set[r.Intn(len(set))])
		}
		return b.String()
	}
	bad := []string{"", "+", "-", ".", "e5", "1e", "1e+", "0x", "0x.p1", "_1", "1_", "1__2", "1._5", "0x_1", "1.2.3", "abc", "1e99999999999999999999", "1e-9223372036854775809", "Inf", "-inf", "+Inf", "INF", "+inf", "infinity", "1p2147483647", "1p-2147483649", "0.5p-2147483647", "1é", "1\x01", "1'", "1\\", "1 ", " 1", "0b102", "0o19", "1e1_0", "1e_10", "0x1p", "1e100000", "1e-100000", "7e-325", "0b1.1p-1", "0o7.7", "0X1.8P+3", "1E-2", "00", "0_0", "-0", "+0.0e0"}
	if r.Intn(4) == 0 {
		return bad[r.Intn(len(bad))]
	}
	sign := []string{"", "", "-", "+"}[r.Intn(4)]
	exp := ""
	if r.Intn(2) == 0 {
		exp = []string{"e", "E", "p", "P"}[r.Intn(4)] + []string{"", "-", "+"}[r.Intn(3)] + fmt.Sprint(r.Intn(400))
	}
	int1 := digits(r.Intn(30), "0123456789")
	frac := ""
	if r.Intn(2) == 0 {
		frac = "." + digits(r.Intn(30), "0123456789")
	}
	switch r.Intn(5) {
	case 0:
		return sign + "0x" + digits(1+r.Intn(20), "0123456789abcdefABCDEF") + "." + digits(r.Intn(10), "0123456789abcdef") + "p" + fmt.Sprint(r.Intn(200)-100)
	case 1:
		return sign + "0b" + digits(1+r.Intn(70), "01") + exp
	case 2:
		return sign + "1_000_" + digits(3, "0123456789") + frac + exp
	}
	if int1 == "" && frac == "" {
		int1 = "0"
	}
	return sign + int1 + frac + exp
}

func bfCorpus() string {
	r := rand.New(rand.NewSource(602))
	var o strings.Builder
	o.WriteString(header("math/big's Float on seeded random operands (every precision up to 600 bits, every rounding mode, zeros, infinities, exponents near the limits): arithmetic, Sqrt, conversions, Text in every format, and ParseFloat on valid and malformed texts"))
	o.WriteString("use std/big\n\n/// (op, a, b, k, Go's result); a and b are operands as FORM:SIGN:MANT:EXP:PREC:MODE (or the op's input), k a precision, a shift or a digit count\nconst OPS: List[(Str, Str, Str, Int, Str)] = [\n")
	row := func(op, a, b string, k int, want string) {
		if len(want)+len(a) > 4000 {
			panic(fmt.Sprintf("bigfloat: a long row: %s %.80s %.80s %d (%d bytes)", op, a, b, k, len(want)+len(a)))
		}
		fmt.Fprintf(&o, "  (%s, %s, %s, %d, %s),\n", xq(op), xq(a), xq(b), k, xq(want))
	}
	newZ := func(x *big.Float, k int) *big.Float {
		z := new(big.Float).SetMode(x.Mode())
		if k > 0 {
			z.SetPrec(uint(k))
		}
		return z
	}
	ops := []string{"add", "sub", "mul", "quo", "sqrt", "prec", "cmp", "pow2", "mantexp", "toint", "tofloat", "tobig", "info", "text"}
	for i := 0; i < 4200; i++ {
		op := ops[i%len(ops)]
		x, xs := bfVal(r, op == "text" || op == "tobig")
		y, ys := bfVal(r, false)
		k := 0
		if r.Intn(3) == 0 {
			k = int(bfPrecs[r.Intn(len(bfPrecs))])
		}
		var want string
		ok := true
		switch op {
		case "add":
			want, ok = bfTry(func() string { return bfRep(newZ(x, k).Add(x, y)) })
		case "sub":
			want, ok = bfTry(func() string { return bfRep(newZ(x, k).Sub(x, y)) })
		case "mul":
			want, ok = bfTry(func() string { return bfRep(newZ(x, k).Mul(x, y)) })
		case "quo":
			want, ok = bfTry(func() string { return bfRep(newZ(x, k).Quo(x, y)) })
		case "sqrt":
			x.Abs(x)
			xs = strings.Replace(xs, ":-:", ":+:", 1)
			want, ok = bfTry(func() string { return bfRep(newZ(x, k).Sqrt(x)) })
		case "prec":
			k = r.Intn(130)
			want = bfRep(new(big.Float).Copy(x).SetPrec(uint(k)))
		case "cmp":
			want = fmt.Sprint(x.Cmp(y))
		case "pow2":
			k = r.Intn(400) - 200
			if r.Intn(5) == 0 {
				k = []int{math.MaxInt32, math.MinInt32, 1 << 32, -(1 << 32)}[r.Intn(4)]
			}
			want = bfRep(new(big.Float).Copy(x).SetMantExp(x, k))
		case "mantexp":
			m := new(big.Float)
			e := x.MantExp(m)
			want = fmt.Sprintf("%s %d", bfRep(m), e)
		case "toint":
			v, a := x.Int64()
			want = fmt.Sprintf("%d %s", v, a)
		case "tofloat":
			if r.Intn(2) == 0 && x.Sign() != 0 && !x.IsInf() {
				// near float64's range and its denormals
				x.SetMantExp(x, []int{-1074, -1075, -1073, -1022, -1021, -1050, 1024, 1023, 1025}[r.Intn(9)]-x.MantExp(nil))
				xs = bfRepIn(x)
			}
			v, a := x.Float64()
			want = fmt.Sprintf("%d %s", int64(math.Float64bits(v)), a)
		case "tobig":
			v, a := x.Int(nil)
			s := "none"
			if v != nil {
				s = v.Text(16)
			}
			want = fmt.Sprintf("%s %s", s, a)
		case "info":
			want = fmt.Sprintf("%d %d %t %t %t", x.MinPrec(), x.Sign(), x.Signbit(), x.IsInf(), x.IsInt())
		case "text":
			f := "eEfgGbpxq"[r.Intn(9)]
			k = []int{-1, -1, 0, 1, 2, 5, 10, 17, 30}[r.Intn(9)]
			ys = string(f)
			want = x.Text(f, k)
		}
		if !ok {
			continue
		}
		row(op, xs, ys, k, want)
	}
	// ParseFloat
	for i := 0; i < 900; i++ {
		s := bfParseInput(r)
		base := []int{0, 0, 0, 10, 2, 8, 16}[r.Intn(7)]
		prec := []int{0, 0, 10, 24, 53, 64, 100, 300}[r.Intn(8)]
		mode := r.Intn(6)
		f, _, err := big.ParseFloat(s, base, uint(prec), big.RoundingMode(mode))
		want := ""
		if err != nil {
			want = "err " + err.Error()
		} else {
			want = bfRep(f)
		}
		row("parse", s, fmt.Sprintf("%d:%d:%d", base, prec, mode), 0, want)
	}
	// SetFloat64, SetInt64, SetInt
	for i := 0; i < 500; i++ {
		var v float64
		switch r.Intn(6) {
		case 0:
			v = math.Float64frombits(uint64(r.Int63n(1 << 52)))
		case 1:
			v = []float64{0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.MaxFloat64, math.SmallestNonzeroFloat64, 0.1, 1.0 / 3}[r.Intn(8)]
		default:
			v = math.Float64frombits(r.Uint64())
		}
		if math.IsNaN(v) {
			v = 2.5
		}
		k := []int{0, 0, 1, 5, 24, 52, 53, 60, 100}[r.Intn(9)]
		var z *big.Float
		if k == 0 {
			z = big.NewFloat(v)
		} else {
			z = new(big.Float).SetPrec(uint(k)).SetFloat64(v)
		}
		row("fromfloat", fmt.Sprint(int64(math.Float64bits(v))), "", k, bfRep(z))
		n := r.Int63() >> uint(r.Intn(63))
		if r.Intn(2) == 0 {
			n = -n
		}
		if r.Intn(20) == 0 {
			n = []int64{0, math.MinInt64, math.MaxInt64, -1, 1}[r.Intn(5)]
		}
		row("fromint", fmt.Sprint(n), "", k, bfRep(new(big.Float).SetPrec(uint(k)).SetInt64(n)))
		b := bigRand(r, []int{10, 64, 100, 300}[r.Intn(4)])
		row("frombig", b.Text(16), "", k, bfRep(new(big.Float).SetPrec(uint(k)).SetInt(b)))
	}
	// Abs keeps the operand's precision and mode, as Go's x.Abs(x) on a
	// copy (a new z would take its own mode), after the others so their
	// rows keep their seeds
	for i := 0; i < 200; i++ {
		x, xs := bfVal(r, false)
		row("abs", xs, "", 0, bfRep(new(big.Float).Copy(x).Abs(x)))
	}
	o.WriteString("]\n")
	o.WriteString(bfTests)
	return o.String()
}

// bfRepIn is x as a corpus operand.
func bfRepIn(x *big.Float) string {
	sign := "+"
	if x.Signbit() {
		sign = "-"
	}
	m := new(big.Float)
	exp := x.MantExp(m)
	m.SetMantExp(m, int(x.MinPrec()))
	i, _ := m.Int(nil)
	return fmt.Sprintf("F:%s:%x:%d:%d:%d", sign, i.Abs(i), exp, x.Prec(), x.Mode())
}

const bfTests = `
const MODES: List[big.RoundingMode] = [big.RoundingMode.ToNearestEven, big.RoundingMode.ToNearestAway, big.RoundingMode.ToZero, big.RoundingMode.AwayFromZero, big.RoundingMode.ToNegativeInf, big.RoundingMode.ToPositiveInf]

fn num(s: Str) -> Int { Int.parse(s) ?? fault("int ${s}") }

/// An operand: FORM:SIGN:MANT:EXP:PREC:MODE.
fn val(s: Str) -> big.BigFloat {
  let p = s.split(":")
  let neg = p[1] == "-"
  let prec = num(p[4])
  let x = if p[0] == "I" {
    big.inf(neg).with_prec(prec)
  } else if p[0] == "Z" {
    let z = big.float_from_int(0, prec: prec)
    if neg { z.neg() } else { z }
  } else {
    let m = big.parse(p[2], base: 16) ?? _e => fault("mant ${p[2]}")
    let f = m.to_big_float(prec: prec).mul_pow2(num(p[3]) - m.bit_len())
    if neg { f.neg() } else { f }
  }
  x.with_mode(MODES[num(p[5])])
}

fn rep(x: big.BigFloat) -> Str {
  "${x.form} ${if x.neg { "-" } else { "+" }} ${x.prec()} ${x.mode()} ${x.acc()} ${x.mant.text(16)} ${x.exp}"
}

fn run(op: Str, a: Str, b: Str, k: Int) -> Str {
  match op {
    "add" => rep(val(a).add(val(b), prec: k))
    "sub" => rep(val(a).sub(val(b), prec: k))
    "mul" => rep(val(a).mul(val(b), prec: k))
    "quo" => rep(val(a).quo(val(b), prec: k))
    "sqrt" => rep(val(a).sqrt(prec: k))
    "prec" => rep(val(a).with_prec(k))
    "cmp" => "${val(a).cmp(val(b))}"
    "abs" => rep(val(a).abs())
    "pow2" => rep(val(a).mul_pow2(k))
    "mantexp" => {
      let (m, e) = val(a).mant_exp()
      "${rep(m)} ${e}"
    }
    "toint" => {
      let (v, acc) = val(a).to_int()
      "${v} ${acc}"
    }
    "tofloat" => {
      let (v, acc) = val(a).to_float()
      "${v.to_bits()} ${acc}"
    }
    "tobig" => {
      let (v, acc) = val(a).to_big()
      "${v.map(fn(i) { i.text(16) }) ?? "none"} ${acc}"
    }
    "info" => {
      let x = val(a)
      "${x.min_prec()} ${x.sign()} ${x.signbit()} ${x.is_inf()} ${x.is_int()}"
    }
    "text" => val(a).text(Int(b.bytes()[0]), k)
    "parse" => {
      let p = b.split(":")
      match big.parse_float(a, base: num(p[0]), prec: num(p[1]), mode: MODES[num(p[2])]) {
        Ok(x) => rep(x)
        Err(e) => "err ${e.msg}"
      }
    }
    "fromfloat" => rep(big.from_float(Float.from_bits(num(a)), prec: k))
    "fromint" => rep(big.float_from_int(num(a), prec: k))
    "frombig" => rep((big.parse(a, base: 16) ?? _e => fault("big ${a}")).to_big_float(prec: k))
    other => fault("op ${other}")
  }
}

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

/// The rows of one operation that differ from Go's (a test per
/// operation, so each has its own time limit: the 6,600 rows as one test
/// passed 60 s on a loaded Mac, tier 68, 2026-11-11).
fn check_op(name: Str) -> List[(Str, Str, Str, Int, Str, Str)] {
  var bad: List[(Str, Str, Str, Int, Str, Str)] = []
  var n = 0
  for (op, a, b, k, want) in OPS {
    if op != name {
      continue
    }
    n += 1
    let got = run(op, a, b, k)
    if got != want {
      bad.push((op, a, b, k, got, want))
    }
  }
  if n == 0 {
    bad.push((name, "", "", 0, "no rows", ""))
  }
  first_bad(bad)
}

test "every row's operation has its test" {
  let tested = ["add", "sub", "mul", "quo", "sqrt", "cmp", "info", "mantexp", "pow2", "prec", "text", "parse", "toint", "tofloat", "tobig", "fromint", "fromfloat", "frombig"]
  var missing: List[Str] = []
  for (op, _a, _b, _k, _w) in OPS {
    if !tested.contains(op) && !missing.contains(op) {
      missing.push(op)
    }
  }
  expect(missing) == []
}

test "Float add is Go's" {
  expect(check_op("add")) == []
}

test "Float sub is Go's" {
  expect(check_op("sub")) == []
}

test "Float mul is Go's" {
  expect(check_op("mul")) == []
}

test "Float quo is Go's" {
  expect(check_op("quo")) == []
}

test "Float sqrt is Go's" {
  expect(check_op("sqrt")) == []
}

test "Float cmp is Go's" {
  expect(check_op("cmp")) == []
}

test "Float info is Go's" {
  expect(check_op("info")) == []
}

test "Float mantexp is Go's" {
  expect(check_op("mantexp")) == []
}

test "Float pow2 is Go's" {
  expect(check_op("pow2")) == []
}

test "Float prec is Go's" {
  expect(check_op("prec")) == []
}

test "Float text is Go's" {
  expect(check_op("text")) == []
}

test "Float parse is Go's" {
  expect(check_op("parse")) == []
}

test "Float toint is Go's" {
  expect(check_op("toint")) == []
}

test "Float tofloat is Go's" {
  expect(check_op("tofloat")) == []
}

test "Float tobig is Go's" {
  expect(check_op("tobig")) == []
}

test "Float fromint is Go's" {
  expect(check_op("fromint")) == []
}

test "Float fromfloat is Go's" {
  expect(check_op("fromfloat")) == []
}

test "Float frombig is Go's" {
  expect(check_op("frombig")) == []
}

test "display is Text('g', 10)" {
  expect(big.from_float(0.1).display()) == "0.1"
  expect(big.float_from_int(2, prec: 200).sqrt().display()) == "1.414213562"
  expect(big.inf(true).display()) == "-Inf"
  expect("${big.float_from_int(-3)}") == "-3"
}
`
