package main

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"strconv"
	"strings"
)

// bigRand is a random integer of up to bits bits, sometimes negative,
// sometimes with long runs of ones or zeros (carries and borrows).
func bigRand(r *rand.Rand, bits int) *big.Int {
	n := r.Intn(bits + 1)
	z := new(big.Int)
	switch r.Intn(4) {
	case 0:
		z.Lsh(big.NewInt(1), uint(n))
		z.Sub(z, big.NewInt(int64(r.Intn(3))))
	case 1:
		z.Lsh(big.NewInt(1), uint(n))
		z.Add(z, big.NewInt(int64(r.Intn(3))))
	default:
		z.Rand(r, new(big.Int).Lsh(big.NewInt(1), uint(n)))
	}
	if r.Intn(3) == 0 {
		z.Neg(z)
	}
	return z
}

func bigHex(z *big.Int) string { return z.Text(16) }

// bigOp is Go's result of op on a and b as the corpus writes it, or "".
func bigOp(op string, a, b *big.Int, k int) string {
	z := new(big.Int)
	switch op {
	case "add":
		return bigHex(z.Add(a, b))
	case "sub":
		return bigHex(z.Sub(a, b))
	case "mul":
		return bigHex(z.Mul(a, b))
	case "quo", "rem", "div", "mod":
		if b.Sign() == 0 {
			return ""
		}
		switch op {
		case "quo":
			return bigHex(z.Quo(a, b))
		case "rem":
			return bigHex(z.Rem(a, b))
		case "div":
			return bigHex(z.Div(a, b))
		}
		return bigHex(z.Mod(a, b))
	case "gcd":
		return bigHex(z.GCD(nil, nil, new(big.Int).Abs(a), new(big.Int).Abs(b)))
	case "modinv":
		if b.Sign() == 0 {
			return ""
		}
		if z.ModInverse(a, b) == nil {
			return "none"
		}
		return bigHex(z)
	case "expmod":
		if b.Sign() == 0 {
			return ""
		}
		e := big.NewInt(int64(k))
		if z.Exp(a, e, b) == nil {
			return ""
		}
		return bigHex(z)
	case "sqrt":
		if a.Sign() < 0 {
			return ""
		}
		return bigHex(z.Sqrt(a))
	case "lsh":
		return bigHex(z.Lsh(a, uint(k)))
	case "rsh":
		return bigHex(z.Rsh(a, uint(k)))
	case "and":
		return bigHex(z.And(a, b))
	case "or":
		return bigHex(z.Or(a, b))
	case "xor":
		return bigHex(z.Xor(a, b))
	case "andnot":
		return bigHex(z.AndNot(a, b))
	case "not":
		return bigHex(z.Not(a))
	case "bit":
		return strconv.Itoa(int(a.Bit(k)))
	case "bitlen":
		return strconv.Itoa(a.BitLen())
	case "tz":
		return strconv.Itoa(int(a.TrailingZeroBits()))
	case "text":
		return a.Text(k)
	case "bytes":
		return fmt.Sprintf("%x", a.Bytes())
	case "toint":
		if !a.IsInt64() {
			return "none"
		}
		return strconv.FormatInt(a.Int64(), 10)
	case "tofloat":
		f, _ := a.Float64()
		return strconv.FormatInt(int64(math.Float64bits(f)), 10)
	case "cmp":
		return strconv.Itoa(a.Cmp(b))
	case "cmpabs":
		return strconv.Itoa(a.CmpAbs(b))
	case "prime":
		return strconv.FormatBool(a.ProbablyPrime(k))
	}
	panic(op)
}

func bigCorpus() string {
	r := rand.New(rand.NewSource(5))
	var o strings.Builder
	o.WriteString(header("math/big's Int on seeded random operands of up to 1,500 bits (runs of ones and zeros, both signs), every std/big operation, and SetString on valid and invalid texts"))
	o.WriteString("use std/big\n\n/// (op, a in hex, b in hex, k: a shift, bit, base, exponent, or rounds; Go's result: hex, a number (a Float as its bits), none)\nconst OPS: List[(Str, Str, Str, Int, Str)] = [\n")
	ops := []string{"add", "sub", "mul", "quo", "rem", "div", "mod", "gcd", "modinv", "expmod", "sqrt", "lsh", "rsh", "and", "or", "xor", "andnot", "not", "bit", "bitlen", "tz", "text", "bytes", "toint", "tofloat", "cmp", "cmpabs", "prime"}
	for i := 0; i < 2600; i++ {
		op := ops[i%len(ops)]
		size := []int{64, 64, 130, 200, 600, 1500}[r.Intn(6)]
		if op == "expmod" || op == "prime" || op == "modinv" {
			size = []int{64, 130, 300}[r.Intn(3)]
		}
		a, b := bigRand(r, size), bigRand(r, []int{64, 130, size}[r.Intn(3)])
		k := 0
		switch op {
		case "lsh", "rsh", "bit":
			k = r.Intn(200)
		case "text":
			k = 2 + r.Intn(61)
		case "expmod":
			k = r.Intn(1000)
		case "prime":
			k = r.Intn(5)
			if r.Intn(2) == 0 {
				// a prime of the size, or a product of two
				a = new(big.Int).Abs(a)
				a.SetBit(a, 0, 1)
				for !a.ProbablyPrime(20) {
					a.Add(a, big.NewInt(2))
				}
				if r.Intn(3) == 0 {
					q := new(big.Int).Abs(bigRand(r, 64))
					q.SetBit(q, 0, 1)
					for !q.ProbablyPrime(20) {
						q.Add(q, big.NewInt(2))
					}
					a.Mul(a, q)
				}
			}
		}
		want := bigOp(op, a, b, k)
		if want == "" {
			continue
		}
		fmt.Fprintf(&o, "  (%q, %q, %q, %d, %q),\n", op, bigHex(a), bigHex(b), k, want)
	}
	// operands past Go's Karatsuba threshold (40 words) and of very
	// different sizes, for mul, quo, rem, text, and sqrt
	for i := 0; i < 160; i++ {
		op := []string{"mul", "mul", "quo", "rem", "text", "sqrt", "sub"}[r.Intn(7)]
		a := bigRand(r, []int{2600, 4000, 8000}[r.Intn(3)])
		b := bigRand(r, []int{2600, 3000, 64, 1300}[r.Intn(4)])
		k := 10
		if op == "text" {
			k = []int{10, 16, 7}[r.Intn(3)]
		}
		want := bigOp(op, a, b, k)
		if want == "" {
			continue
		}
		fmt.Fprintf(&o, "  (%q, %q, %q, %d, %q),\n", op, bigHex(a), bigHex(b), k, want)
	}
	o.WriteString("]\n\n/// (text, base, Go's SetString: the value in hex, or \"bad\")\nconst PARSE: List[(Str, Int, Str)] = [\n")
	texts := []string{"", "0", "-0", "+7", "--1", "12x", "0x_1f", "0x1f", "0X1F", "0b101", "0o17", "017", "08", "0_1", "1__2", "1_2", "_1", "1_", "0x", "0b", "ff", "FF", "zz", "ZZ", "-", "+", " 1", "1 ", "9223372036854775808", "-9223372036854775809", "1.5"}
	for i := 0; i < 400; i++ {
		var s string
		if i < len(texts) {
			s = texts[i]
		} else {
			z := bigRand(r, 300)
			s = z.Text(10)
			if r.Intn(4) == 0 {
				b := []byte(s)
				b[r.Intn(len(b))] = "_x.9-+ "[r.Intn(7)]
				s = string(b)
			}
		}
		for _, base := range []int{0, 10, 16, 2, 36, 62} {
			if i >= len(texts) && base != 10 && base != 0 {
				continue
			}
			z, ok := new(big.Int).SetString(s, base)
			w := "bad"
			if ok {
				w = bigHex(z)
			}
			fmt.Fprintf(&o, "  (%s, %d, %q),\n", xq(s), base, w)
		}
	}
	o.WriteString("]\n")
	o.WriteString(bigTests)
	return o.String()
}

const bigTests = `
fn h(s: Str) -> big.BigInt { big.parse(s, base: 16).or_fault("corpus hex") }

fn hx(x: big.BigInt) -> Str { x.text(base: 16) }

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn run(op: Str, a: big.BigInt, b: big.BigInt, k: Int) -> Str {
  match op {
    "add" => hx(a.add(b))
    "sub" => hx(a.sub(b))
    "mul" => hx(a.mul(b))
    "quo" => hx(a.quo(b))
    "rem" => hx(a.rem(b))
    "div" => hx(a.div(b))
    "mod" => hx(a.mod(b))
    "gcd" => hx(a.gcd(b))
    "modinv" => a.mod_inverse(b).map(hx) ?? "none"
    "expmod" => hx(a.exp(big.from_int(k), m: Some(b)))
    "sqrt" => hx(a.sqrt())
    "lsh" => hx(a.lsh(k))
    "rsh" => hx(a.rsh(k))
    "and" => hx(a.and(b))
    "or" => hx(a.or(b))
    "xor" => hx(a.xor(b))
    "andnot" => hx(a.and_not(b))
    "not" => hx(a.not())
    "bit" => "${a.bit(k)}"
    "bitlen" => "${a.bit_len()}"
    "tz" => "${a.trailing_zeros()}"
    "text" => a.text(base: k)
    "bytes" => a.bytes().to_hex()
    "toint" => a.to_int().map(fn(n) { "${n}" }) ?? "none"
    "tofloat" => "${a.to_float().to_bits()}"
    "cmp" => "${a.cmp(b)}"
    "cmpabs" => "${a.cmp_abs(b)}"
    _ => "${a.probably_prime(k)}"
  }
}

test "every operation gives Go's math/big result" {
  var bad: List[(Str, Str, Str, Int, Str, Str)] = []
  for (op, a, b, k, want) in OPS {
    let got = run(op, h(a), h(b), k)
    if got != want {
      bad.push((op, a, b, k, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "the order of BigInt is the numbers' order" {
  var bad: List[(Str, Str)] = []
  for (op, a, b, _k, want) in OPS {
    if op != "cmp" {
      continue
    }
    let x = h(a)
    let y = h(b)
    let got = if x < y { "-1" } else if x == y { "0" } else { "1" }
    if got != want {
      bad.push((a, b))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse accepts and reads what Go's SetString does" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (s, base, want) in PARSE {
    let got = match big.parse(s, base: base) {
      Ok(x) => hx(x)
      Err(_) => "bad"
    }
    if got != want {
      bad.push((s, base, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
