package main

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"math/rand"
	"strings"
)

// The API this corpus covers, as std.md 2.1 and decisions 0260 and 0287
// declare it: Int's word methods — leading_zeros, trailing_zeros, ones_count,
// wrapping_shl, shr_logical, rotate_right, rotate_right32, swap_bytes,
// mul_hi_unsigned, lt_unsigned, le_unsigned — then the wrapping, saturating
// and checked arithmetic. Every expectation is Go's math/bits on the same 64
// bit word (the package itself, not only the tables of its tests, so the
// inputs can go past what those tables list), and Go's plain arithmetic with
// a guard where the operation can overflow: checked returns None (std.md 2.1
// says Int?), saturating clamps to the ends of Int, and wrapping keeps the low
// 64 bits. Each row says the rule it follows.

// xoWord writes an Int literal for a 64 bit pattern: MinInt64 as an
// expression, since -9223372036854775808 does not fit Int on its own.
func xoWord(v int64) string {
	if v == math.MinInt64 {
		return "(-9223372036854775807 - 1)"
	}
	return fmt.Sprintf("%d", v)
}

// wordInputs are the words the cases run over: the shapes a 64 bit word
// takes, then seeded ones.
func wordInputs() []int64 {
	ws := []int64{0, 1, -1, 2, -2, 3, 7, 8, 15, 16, 31, 32, 33, 63, 64, 65,
		127, 128, 255, 256, 1024, 4096, 1 << 20, 1 << 40, 1 << 62,
		math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1,
		0x5555555555555555, -6148914691236517206, 0x0F0F0F0F0F0F0F0F, -1085102592571150096,
		0x0123456789ABCDEF, 0x7F7F7F7F7F7F7F7F, -9223372036854775807, -1000000007}
	for i := 0; i < 60; i++ {
		ws = append(ws, int64(1)<<uint(i%64))
		ws = append(ws, (int64(1)<<uint(i%64))-1)
	}
	rw := rand.New(rand.NewSource(131))
	for i := 0; i < 150; i++ {
		ws = append(ws, rw.Int63()-rw.Int63())
	}
	return ws
}

// wordShifts are the shift and rotate counts: inside 64, at its ends, and
// past them, which std.md 2.1 takes mod 64 (and rotate_right32 mod 32).
func wordShifts() []int64 {
	return []int64{0, 1, 2, 7, 8, 15, 16, 30, 31, 32, 33, 47, 48, 62, 63, 64, 65, 100, 127}
}

func wordsCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's math/bits on 64 bit words, and Go's arithmetic with an overflow guard, for Int's word methods (decision 0260) and its wrapping, saturating and checked arithmetic (std.md 2.1, decision 0287): the inputs are the shapes of a 64 bit word, every power of two and one below it, and seeded words; the counts run inside, at the ends of and past 64. The bit tables of Go's own bits_test.go are the same shapes this generator covers through math/bits itself (Go's math/bits tests are Copyright 2009 The Go Authors, BSD-style license)"))

	ws := wordInputs()
	const lab = "std.md 2.1 word methods (decision 0260)"

	o.WriteString("\n/// (word, math/bits.LeadingZeros64, TrailingZeros64, OnesCount64, the rule)\nconst BIT_CASES: List[(Int, Int, Int, Int, Str)] = [\n")
	for _, w := range ws {
		u := uint64(w)
		fmt.Fprintf(&o, "  (%s, %d, %d, %d, %q),\n", xoWord(w),
			bits.LeadingZeros64(u), bits.TrailingZeros64(u), bits.OnesCount64(u), lab)
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (word, math/bits.ReverseBytes64, the rule)\nconst SWAP_CASES: List[(Int, Int, Str)] = [\n")
	for _, w := range ws {
		fmt.Fprintf(&o, "  (%s, %s, %q),\n", xoWord(w),
			xoWord(int64(bits.ReverseBytes64(uint64(w)))), lab)
	}
	o.WriteString("]\n")

	// Shifts and rotates: k mod 64 (mod 32 for the 32 bit rotate, which is
	// zero extended), the wrapping shift dropping what leaves the word, and
	// the logical shift filling zeros.
	o.WriteString("\n/// (word, k, RotateLeft64(word, -k) for a right rotate, RotateLeft32(low 32, -k) zero extended, the logical right shift, the wrapping left shift, the rule)\nconst ROT_CASES: List[(Int, Int, Int, Int, Int, Int, Str)] = [\n")
	ks := wordShifts()
	for i, w := range ws {
		for _, k := range []int64{ks[i%len(ks)], ks[(i+7)%len(ks)]} {
			rr := int64(bits.RotateLeft64(uint64(w), -int(k%64)))
			rr32 := int64(bits.RotateLeft32(uint32(w), -int(k%32)))
			shr := int64(uint64(w) >> uint(k%64))
			shl := int64(uint64(w) << uint(k%64))
			fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s, %s, %q),\n", xoWord(w), xoWord(k),
				xoWord(rr), xoWord(rr32), xoWord(shr), xoWord(shl), lab)
		}
	}
	o.WriteString("]\n")

	// mul_hi_unsigned: the high word of the 128 bit product, as unsigned
	// words, so the value is the same 64 bit pattern read as an Int.
	o.WriteString("\n/// (a, b, the high word of math/bits.Mul64, the rule)\nconst MULHI_CASES: List[(Int, Int, Int, Str)] = [\n")
	pairs := []struct{ a, b int64 }{
		{0, 0}, {1, 1}, {-1, -1}, {math.MaxInt64, math.MaxInt64},
		{math.MinInt64, math.MinInt64}, {-1, 1}, {math.MinInt64, -1},
		{0x0123456789ABCDEF, 0x0FEDCBA987654321}, {1 << 32, 1 << 32},
		{(1 << 32) - 1, (1 << 32) - 1}, {math.MaxInt64, 2}, {math.MinInt64, 3},
	}
	rp := rand.New(rand.NewSource(137))
	for i := 0; i < 40; i++ {
		pairs = append(pairs, struct{ a, b int64 }{rp.Int63() - rp.Int63(), rp.Int63() - rp.Int63()})
	}
	for _, p := range pairs {
		hi, _ := bits.Mul64(uint64(p.a), uint64(p.b))
		fmt.Fprintf(&o, "  (%s, %s, %s, %q),\n", xoWord(p.a), xoWord(p.b), xoWord(int64(hi)), lab)
	}
	o.WriteString("]\n")

	// Unsigned compares of the same two words.
	o.WriteString("\n/// (a, b, a < b unsigned, a <= b unsigned, the rule)\nconst CMP_CASES: List[(Int, Int, Bool, Bool, Str)] = [\n")
	for _, p := range pairs {
		ua, ub := uint64(p.a), uint64(p.b)
		fmt.Fprintf(&o, "  (%s, %s, %v, %v, %q),\n", xoWord(p.a), xoWord(p.b), ua < ub, ua <= ub, lab)
	}
	o.WriteString("]\n")

	// Saturating: clamped to the ends of Int.
	const satLab = "std.md 2.1 saturating: clamped to the ends of Int"
	o.WriteString("\n/// (a, b, saturating_add, saturating_sub, saturating_mul, the rule)\nconst SAT_CASES: List[(Int, Int, Int, Int, Int, Str)] = [\n")
	for _, p := range pairs {
		add := satAdd(p.a, p.b)
		sub := satSub(p.a, p.b)
		mul := satMul(p.a, p.b)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s, %q),\n", xoWord(p.a), xoWord(p.b),
			xoWord(add), xoWord(sub), xoWord(mul), satLab)
	}
	o.WriteString("]\n")

	// Checked: None on overflow (std.md 2.1 says Int?), and checked_div is
	// None for a zero divisor and for MinInt64 / -1.
	const chkLab = "std.md 2.1 checked: None on overflow"
	const divLab = "std.md 2.1 checked_div: None on a zero divisor or MinInt64 / -1"
	o.WriteString("\n/// (a, b, checked_add, checked_sub, checked_mul, checked_div, the rules; \"none\" for None)\nconst CHK_CASES: List[(Int, Int, Str, Str, Str, Str, Str)] = [\n")
	for _, p := range pairs {
		fmt.Fprintf(&o, "  (%s, %s, %q, %q, %q, %q, %q),\n", xoWord(p.a), xoWord(p.b),
			optText(chkAdd(p.a, p.b)), optText(chkSub(p.a, p.b)),
			optText(chkMul(p.a, p.b)), optText(chkDiv(p.a, p.b)),
			chkLab+"; "+divLab)
	}
	o.WriteString("]\n")

	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "leading_zeros, trailing_zeros and ones_count are math/bits" {
  var bad: List[(Str, Str, Str)] = []
  for (w, lz, tz, ones, _) in BIT_CASES {
    let got = "${w.leading_zeros()} ${w.trailing_zeros()} ${w.ones_count()}"
    let want = "${lz} ${tz} ${ones}"
    if got != want {
      bad.push(("word ${w}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "swap_bytes reverses the eight bytes" {
  var bad: List[(Str, Str, Str)] = []
  for (w, want, _) in SWAP_CASES {
    let got = "${w.swap_bytes()}"
    if got != "${want}" {
      bad.push(("word ${w}", got, "${want}"))
    }
  }
  expect(show3(bad)) == []
}

test "the shifts and rotates take their count mod 64, and 32 for rotate_right32" {
  var bad: List[(Str, Str, Str)] = []
  for (w, k, rr, rr32, shr, shl, _) in ROT_CASES {
    let got = "${w.rotate_right(k)} ${w.rotate_right32(k)} ${w.shr_logical(k)} ${w.wrapping_shl(k)}"
    let want = "${rr} ${rr32} ${shr} ${shl}"
    if got != want {
      bad.push(("word ${w} by ${k}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "mul_hi_unsigned is the high word of the 128 bit product" {
  var bad: List[(Str, Str, Str)] = []
  for (a, b, want, _) in MULHI_CASES {
    let got = "${a.mul_hi_unsigned(b)}"
    if got != "${want}" {
      bad.push(("${a} times ${b}", got, "${want}"))
    }
  }
  expect(show3(bad)) == []
}

test "lt_unsigned and le_unsigned compare the words unsigned" {
  var bad: List[(Str, Str, Str)] = []
  for (a, b, lt, le, _) in CMP_CASES {
    let got = "${a.lt_unsigned(b)} ${a.le_unsigned(b)}"
    let want = "${lt} ${le}"
    if got != want {
      bad.push(("${a} against ${b}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "saturating arithmetic clamps to the ends of Int" {
  var bad: List[(Str, Str, Str)] = []
  for (a, b, add, sub, mul, _) in SAT_CASES {
    let got = "${a.saturating_add(b)} ${a.saturating_sub(b)} ${a.saturating_mul(b)}"
    let want = "${add} ${sub} ${mul}"
    if got != want {
      bad.push(("${a} with ${b}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "checked arithmetic is None on overflow, and checked_div on a zero divisor" {
  var bad: List[(Str, Str, Str)] = []
  for (a, b, add, sub, mul, div, _) in CHK_CASES {
    let got = "${opt_text(a.checked_add(b))} ${opt_text(a.checked_sub(b))} ${opt_text(a.checked_mul(b))} ${opt_text(a.checked_div(b))}"
    let want = "${add} ${sub} ${mul} ${div}"
    if got != want {
      bad.push(("${a} with ${b}", got, want))
    }
  }
  expect(show3(bad)) == []
}

fn opt_text(n: Int?) -> Str {
  match n {
    Some(x) => "${x}"
    None => "none"
  }
}
`)
	return o.String()
}

// optText is the corpus's text for an Option Int.
func optText(n int64, ok bool) string {
	if !ok {
		return "none"
	}
	return fmt.Sprintf("%d", n)
}

// satAdd, satSub and satMul clamp to the ends of Int, as saturating_add,
// saturating_sub and saturating_mul do (std.md 2.1). The products and sums are
// exact in big.Int, so the clamps cannot be wrong.
func bigOf(a int64) *big.Int { return big.NewInt(a) }

func clamp(v *big.Int) int64 {
	if v.Cmp(big.NewInt(math.MaxInt64)) > 0 {
		return math.MaxInt64
	}
	if v.Cmp(big.NewInt(math.MinInt64)) < 0 {
		return math.MinInt64
	}
	return v.Int64()
}

func fits(v *big.Int) (int64, bool) {
	if v.Cmp(big.NewInt(math.MaxInt64)) > 0 || v.Cmp(big.NewInt(math.MinInt64)) < 0 {
		return 0, false
	}
	return v.Int64(), true
}

func satAdd(a, b int64) int64 { return clamp(new(big.Int).Add(bigOf(a), bigOf(b))) }
func satSub(a, b int64) int64 { return clamp(new(big.Int).Sub(bigOf(a), bigOf(b))) }
func satMul(a, b int64) int64 { return clamp(new(big.Int).Mul(bigOf(a), bigOf(b))) }

func chkAdd(a, b int64) (int64, bool) { return fits(new(big.Int).Add(bigOf(a), bigOf(b))) }
func chkSub(a, b int64) (int64, bool) { return fits(new(big.Int).Sub(bigOf(a), bigOf(b))) }
func chkMul(a, b int64) (int64, bool) { return fits(new(big.Int).Mul(bigOf(a), bigOf(b))) }

func chkDiv(a, b int64) (int64, bool) {
	// std.md 2.1's checked_div: None on a zero divisor, and on MinInt64 / -1,
	// which has no Int as its quotient
	if b == 0 || (b == -1 && a == math.MinInt64) {
		return 0, false
	}
	return a / b, true
}
