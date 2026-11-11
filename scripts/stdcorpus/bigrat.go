package main

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"strings"
)

// brVal is a random Rat operand: zero, an integer, a small fraction, a
// fraction of large random parts, or a denominator of 2s and 5s (finite
// decimals) times a small odd factor.
func brVal(r *rand.Rand) *big.Rat {
	neg := r.Intn(2) == 0
	var a, b *big.Int
	switch k := r.Intn(12); {
	case k == 0:
		return new(big.Rat)
	case k == 1:
		a, b = bigRand(r, 200), big.NewInt(1)
	case k <= 4:
		a, b = big.NewInt(r.Int63n(1000)), big.NewInt(1+r.Int63n(1000))
	case k <= 7:
		a, b = bigRand(r, 1+r.Intn(300)), bigRand(r, 1+r.Intn(300))
	default:
		a = bigRand(r, 1+r.Intn(120))
		b = new(big.Int).Lsh(big.NewInt(1), uint(r.Intn(70)))
		b.Mul(b, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(r.Intn(70))), nil))
		b.Mul(b, big.NewInt([]int64{1, 1, 1, 3, 7, 11}[r.Intn(6)]))
	}
	a.Abs(a)
	b.Abs(b)
	if b.Sign() == 0 {
		b.SetInt64(1)
	}
	if neg {
		a.Neg(a)
	}
	return new(big.Rat).SetFrac(a, b)
}

// brParseInput is a text for Rat.UnmarshalText: fractions, decimals with
// exponents in every base, and malformed ones.
func brParseInput(r *rand.Rand) string {
	digits := func(n int, set string) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(set[r.Intn(len(set))])
		}
		return b.String()
	}
	bad := []string{"", "+", "-", ".", "/", "1/", "/2", "1/0", "1/00", "0/0", "1/-2", "1/+2", "-1/2", "+3/6", "1/2/3", "1/2.5", "1.5/2", "1/0x10", "1/010", "1/08", "1/0b11", "1/0o17", "0x10/3", "1_0/5", "1/1_0", "1/_1", "e5", "1e", "1e+", "1e-", "0x", "0x.p1", "_1", "1_", "1__2", "1._5", "0x_1", "1.2.3", "abc", "1e99999999999999999999", "1e-9223372036854775808", "1e9223372036854775807", "1e1000001", "1e-1000001", "1p10000001", "1p-10000001", "1e1_0", "1e_10", "1e10_", "1p", "0x1p", "0b1.1p-1", "0o7.7", "0X1.8P+3", "1E-2", "00", "0_0", "-0", "+0.0e0", "0.000", "Inf", "NaN", " 1", "1 ", "1é", "1/2 ", "0x1.8e3", "0b102", "0o19", "010.5", "0.1e-2p3"}
	if r.Intn(4) == 0 {
		return bad[r.Intn(len(bad))]
	}
	sign := []string{"", "", "-", "+"}[r.Intn(4)]
	exp := ""
	if r.Intn(2) == 0 {
		exp = []string{"e", "E", "p", "P"}[r.Intn(4)] + []string{"", "-", "+"}[r.Intn(3)] + fmt.Sprint(r.Intn(60))
	}
	int1 := digits(r.Intn(25), "0123456789")
	frac := ""
	if r.Intn(2) == 0 {
		frac = "." + digits(r.Intn(25), "0123456789")
	}
	switch r.Intn(7) {
	case 0:
		return sign + "0x" + digits(1+r.Intn(16), "0123456789abcdefABCDEF") + "." + digits(r.Intn(8), "0123456789abcdef") + "p" + fmt.Sprint(r.Intn(200)-100)
	case 1:
		return sign + "0b" + digits(1+r.Intn(60), "01") + exp
	case 2:
		return sign + "1_000_" + digits(3, "0123456789") + frac + exp
	case 3:
		return sign + digits(1+r.Intn(30), "0123456789") + "/" + []string{"", "0x", "0", "0b", "0o"}[r.Intn(5)] + digits(1+r.Intn(20), "01234567")
	case 4:
		return sign + "0o" + digits(1+r.Intn(20), "01234567") + frac[:min(len(frac), 4)] + exp
	}
	if int1 == "" && frac == "" {
		int1 = "0"
	}
	return sign + int1 + frac + exp
}

func brCorpus() string {
	r := rand.New(rand.NewSource(608))
	var o strings.Builder
	o.WriteString(header("math/big's Rat on seeded random operands (zero, integers, small and large fractions, finite decimals): arithmetic, Cmp, Float64, FloatPrec, String, RatString, FloatString, SetFloat64, SetFrac, SetString as UnmarshalText, Float.SetRat and Float.Rat"))
	o.WriteString("use std/big\n\n/// (op, a, b, k, Go's result); a and b are operands as num/den (or the op's input), k a precision, digit count or mode\nconst OPS: List[(Str, Str, Str, Int, Str)] = [\n")
	row := func(op, a, b string, k int, want string) {
		if len(want)+len(a)+len(b) > 4000 {
			panic(fmt.Sprintf("bigrat: a long row: %s %.80s %.80s %d", op, a, b, k))
		}
		fmt.Fprintf(&o, "  (%s, %s, %s, %d, %s),\n", xq(op), xq(a), xq(b), k, xq(want))
	}
	ops := []string{"add", "sub", "mul", "quo", "neg", "abs", "inv", "cmp", "info", "tofloat", "floatprec", "text", "tobigfloat"}
	for i := 0; i < 3900; i++ {
		op := ops[i%len(ops)]
		x, y := brVal(r), brVal(r)
		xs, ys := x.String(), y.String()
		k := 0
		var want string
		switch op {
		case "add":
			want = new(big.Rat).Add(x, y).String()
		case "sub":
			want = new(big.Rat).Sub(x, y).String()
		case "mul":
			if r.Intn(8) == 0 {
				y, ys = x, xs
			}
			want = new(big.Rat).Mul(x, y).String()
		case "quo":
			if y.Sign() == 0 {
				y, ys = big.NewRat(7, 3), "7/3"
			}
			want = new(big.Rat).Quo(x, y).String()
		case "neg":
			want = new(big.Rat).Neg(x).String()
		case "abs":
			want = new(big.Rat).Abs(x).String()
		case "inv":
			if x.Sign() == 0 {
				x, xs = big.NewRat(-5, 2), "-5/2"
			}
			want = new(big.Rat).Inv(x).String()
		case "cmp":
			if r.Intn(6) == 0 {
				y, ys = new(big.Rat).Set(x), xs
			}
			want = fmt.Sprint(x.Cmp(y))
		case "info":
			want = fmt.Sprintf("%d %t %s %s", x.Sign(), x.IsInt(), x.Num(), x.Denom())
		case "tofloat":
			if r.Intn(2) == 0 {
				// near float64's range, its denormals and its halfway points
				f := math.Float64frombits(r.Uint64() &^ (1 << 63))
				if math.IsNaN(f) || math.IsInf(f, 0) {
					f = 1.5
				}
				q, _ := new(big.Rat).SetString([]string{"0", "1/3", "-1/3", "1/1000000", "1"}[r.Intn(5)])
				q.Mul(q, new(big.Rat).SetFloat64(math.Nextafter(f, 0)-f))
				x = new(big.Rat).Add(new(big.Rat).SetFloat64(f), q)
				switch r.Intn(6) {
				case 0:
					x.SetFrac(big.NewInt(int64(1+r.Intn(5))), new(big.Int).Lsh(big.NewInt(1), uint(1070+r.Intn(10))))
				case 1:
					x.SetFrac(new(big.Int).Lsh(big.NewInt(int64(1+r.Intn(5))), uint(1020+r.Intn(10))), big.NewInt(int64(1+r.Intn(3))))
				}
				xs = x.String()
			}
			f, ex := x.Float64()
			want = fmt.Sprintf("%d %t", int64(math.Float64bits(f)), ex)
		case "floatprec":
			n, ex := x.FloatPrec()
			want = fmt.Sprintf("%d %t", n, ex)
		case "text":
			k = []int{-1, 0, 0, 1, 2, 3, 5, 10, 20, 40}[r.Intn(10)]
			want = x.String() + "|" + x.RatString() + "|" + x.FloatString(k)
		case "tobigfloat":
			prec := []int{0, 0, 1, 2, 10, 24, 53, 64, 65, 100, 200}[r.Intn(11)]
			mode := r.Intn(6)
			k = prec*8 + mode
			want = bfRep(new(big.Float).SetPrec(uint(prec)).SetMode(big.RoundingMode(mode)).SetRat(x))
		}
		row(op, xs, ys, k, want)
	}
	// SetString (as UnmarshalText, for Go's error text)
	for i := 0; i < 900; i++ {
		s := brParseInput(r)
		z := new(big.Rat)
		want := ""
		if err := z.UnmarshalText([]byte(s)); err != nil {
			want = "err " + err.Error()
		} else {
			want = z.String()
		}
		row("parse", s, "", 0, want)
	}
	// SetFloat64, SetInt64, SetFrac, Float.Rat
	for i := 0; i < 400; i++ {
		var v float64
		switch r.Intn(5) {
		case 0:
			v = math.Float64frombits(uint64(r.Int63n(1 << 52)))
		case 1:
			v = []float64{0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN(), math.MaxFloat64, math.SmallestNonzeroFloat64, 0.1, 1.0 / 3, -2.5, 1e300, 1e-300}[r.Intn(12)]
		default:
			v = math.Float64frombits(r.Uint64())
		}
		z := new(big.Rat).SetFloat64(v)
		want := "none"
		if z != nil {
			want = z.String()
		}
		row("fromfloat", fmt.Sprint(int64(math.Float64bits(v))), "", 0, want)
		n := r.Int63() >> uint(r.Intn(63))
		if r.Intn(2) == 0 {
			n = -n
		}
		if r.Intn(20) == 0 {
			n = []int64{0, math.MinInt64, math.MaxInt64, -1, 1}[r.Intn(5)]
		}
		row("fromint", fmt.Sprint(n), "", 0, big.NewRat(n, 1).String())
		a, b := bigRand(r, 1+r.Intn(200)), bigRand(r, 1+r.Intn(200))
		if r.Intn(2) == 0 {
			a.Neg(a)
		}
		if r.Intn(2) == 0 {
			b.Neg(b)
		}
		if b.Sign() == 0 {
			b.SetInt64(-6)
		}
		if r.Intn(4) == 0 {
			g := bigRand(r, 1+r.Intn(60))
			if g.Sign() != 0 {
				a.Mul(a, g)
				b.Mul(b, g)
			}
		}
		row("frac", a.String(), b.String(), 0, new(big.Rat).SetFrac(a, b).String())
		f, fs := bfVal(r, true)
		if r.Intn(25) == 0 {
			f, fs = new(big.Float).SetInf(n < 0), fmt.Sprintf("I:%s:0:0:53:0", map[bool]string{true: "-", false: "+"}[n < 0])
		}
		q, acc := f.Rat(nil)
		s := "none"
		if q != nil {
			s = q.String()
		}
		row("floattorat", fs, "", 0, s+" "+acc.String())
	}
	o.WriteString("]\n")
	o.WriteString(brTests)
	return o.String()
}

const brTests = `
const MODES: List[big.RoundingMode] = [big.RoundingMode.ToNearestEven, big.RoundingMode.ToNearestAway, big.RoundingMode.ToZero, big.RoundingMode.AwayFromZero, big.RoundingMode.ToNegativeInf, big.RoundingMode.ToPositiveInf]

fn num(s: Str) -> Int { Int.parse(s) ?? fault("int ${s}") }

fn bi(s: Str) -> big.BigInt { big.parse(s) ?? _e => fault("big ${s}") }

/// An operand: num/den as Go's String writes it.
fn val(s: Str) -> big.BigRat {
  let p = s.split("/")
  big.rat(bi(p[0]), bi(p[1]))
}

/// A BigFloat operand: FORM:SIGN:MANT:EXP:PREC:MODE.
fn fval(s: Str) -> big.BigFloat {
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

fn frep(x: big.BigFloat) -> Str {
  "${x.form} ${if x.neg { "-" } else { "+" }} ${x.prec()} ${x.mode()} ${x.acc()} ${x.mant.text(16)} ${x.exp}"
}

fn run(op: Str, a: Str, b: Str, k: Int) -> Str {
  match op {
    "add" => val(a).add(val(b)).text()
    "sub" => val(a).sub(val(b)).text()
    "mul" => val(a).mul(val(b)).text()
    "quo" => val(a).quo(val(b)).text()
    "neg" => val(a).neg().text()
    "abs" => val(a).abs().text()
    "inv" => val(a).inv().text()
    "cmp" => "${val(a).cmp(val(b))}"
    "info" => {
      let x = val(a)
      "${x.sign()} ${x.is_int()} ${x.num()} ${x.denom()}"
    }
    "tofloat" => {
      let (f, ex) = val(a).to_float()
      "${f.to_bits()} ${ex}"
    }
    "floatprec" => {
      let (n, ex) = val(a).float_prec()
      "${n} ${ex}"
    }
    "text" => {
      let x = val(a)
      "${x.text()}|${x.rat_text()}|${x.float_text(k)}"
    }
    "tobigfloat" => frep(val(a).to_big_float(prec: k / 8, mode: MODES[k % 8]))
    "parse" => match big.parse_rat(a) {
      Ok(x) => x.text()
      Err(e) => "err ${e.msg}"
    }
    "fromfloat" => big.rat_from_float(Float.from_bits(num(a))).map(fn(x) { x.text() }) ?? "none"
    "fromint" => big.rat_from_int(num(a)).text()
    "frac" => big.rat(bi(a), bi(b)).text()
    "floattorat" => {
      let (q, acc) = fval(a).to_rat()
      "${q.map(fn(x) { x.text() }) ?? "none"} ${acc}"
    }
    other => fault("op ${other}")
  }
}

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "Rat operations are Go's" {
  var bad: List[(Str, Str, Str, Int, Str, Str)] = []
  for (op, a, b, k, want) in OPS {
    let got = run(op, a, b, k)
    if got != want {
      bad.push((op, a, b, k, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "BigRat values" {
  let third = big.rat(big.from_int(2), big.from_int(-6))
  expect(third.text()) == "-1/3"
  expect("${third}") == "-1/3"
  expect(third == big.rat_from_int(-1).quo(big.rat_from_int(3))) == true
  expect(big.from_int(7).to_rat().rat_text()) == "7"
  expect(big.rat_from_float(0.1).map(fn(x) { x.float_text(20) })) == Some("0.10000000000000000555")
  expect(big.rat_from_float(Float.nan)) == None
}
`
