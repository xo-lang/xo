package rt

// std/big (std.md 33, decision 0311): Go's math/big Int as an immutable
// value. A BigInt holds its ordered key (std/big.xo); each operation
// decodes the operands into *big.Int, calls math/big, and encodes the
// result, so values are never shared with math/big's mutable ones.

import (
	"math"
	"math/big"
	"math/bits"
	"strconv"
)

const bigSignBit = math.MinInt64

func (e Big_BigErr) M_display(*Task) string { return e.F_msg }

// bigOf decodes x's key.
func bigOf(x Big_BigInt) *big.Int {
	k := List_Slice(x.F_key)
	if len(k) == 0 || k[0] == 0 {
		return new(big.Int)
	}
	n := k[0]
	neg := n < 0
	if neg {
		n = -n
	}
	ws := make([]big.Word, n)
	for i := int64(0); i < n; i++ {
		v := k[1+i]
		if neg {
			v ^= math.MaxInt64
		} else {
			v ^= bigSignBit
		}
		ws[n-1-i] = big.Word(uint64(v))
	}
	z := new(big.Int).SetBits(ws)
	if neg {
		z.Neg(z)
	}
	return z
}

// bigKey encodes z.
func bigKey(z *big.Int) Big_BigInt {
	if bits.UintSize != 64 {
		panic("std/big: a 64 bit platform is required")
	}
	ws := z.Bits()
	n := len(ws)
	if z.Sign() == 0 {
		return Big_BigInt{F_key: ListFromSlice([]int64{0})}
	}
	k := make([]int64, 1+n)
	neg := z.Sign() < 0
	if neg {
		k[0] = -int64(n)
	} else {
		k[0] = int64(n)
	}
	for i := 0; i < n; i++ {
		v := int64(uint64(ws[n-1-i]))
		if neg {
			v ^= math.MaxInt64
		} else {
			v ^= bigSignBit
		}
		k[1+i] = v
	}
	return Big_BigInt{F_key: ListFromSlice(k)}
}

func bigZeroFault(y *big.Int) {
	if y.Sign() == 0 {
		FaultHere("division by zero")
	}
}

func Big_from_int(_ *Task, n int64) Big_BigInt { return bigKey(big.NewInt(n)) }

func Big_parse(_ *Task, s string, base int64) Result[Big_BigInt, Big_BigErr] {
	if base != 0 && (base < 2 || base > big.MaxBase) {
		return Err[Big_BigInt](Big_BigErr{F_msg: "invalid base " + strconv.FormatInt(base, 10)})
	}
	z, ok := new(big.Int).SetString(s, int(base))
	if !ok {
		return Err[Big_BigInt](Big_BigErr{F_msg: "invalid number " + strconv.Quote(textOf(s)) + " in base " + strconv.FormatInt(base, 10)})
	}
	return Ok[Big_BigInt, Big_BigErr](bigKey(z))
}

func Big_from_bytes(_ *Task, b Bytes) Big_BigInt {
	return bigKey(new(big.Int).SetBytes([]byte(b)))
}

func (x Big_BigInt) M_add(_ *Task, y Big_BigInt) Big_BigInt {
	return bigKey(new(big.Int).Add(bigOf(x), bigOf(y)))
}

func (x Big_BigInt) M_sub(_ *Task, y Big_BigInt) Big_BigInt {
	return bigKey(new(big.Int).Sub(bigOf(x), bigOf(y)))
}

func (x Big_BigInt) M_mul(_ *Task, y Big_BigInt) Big_BigInt {
	return bigKey(new(big.Int).Mul(bigOf(x), bigOf(y)))
}

func (x Big_BigInt) M_quo(_ *Task, y Big_BigInt) Big_BigInt {
	b := bigOf(y)
	bigZeroFault(b)
	return bigKey(new(big.Int).Quo(bigOf(x), b))
}

func (x Big_BigInt) M_rem(_ *Task, y Big_BigInt) Big_BigInt {
	b := bigOf(y)
	bigZeroFault(b)
	return bigKey(new(big.Int).Rem(bigOf(x), b))
}

func (x Big_BigInt) M_quo_rem(_ *Task, y Big_BigInt) Tuple2[Big_BigInt, Big_BigInt] {
	b := bigOf(y)
	bigZeroFault(b)
	q, r := new(big.Int).QuoRem(bigOf(x), b, new(big.Int))
	return Tuple2[Big_BigInt, Big_BigInt]{F0: bigKey(q), F1: bigKey(r)}
}

func (x Big_BigInt) M_div(_ *Task, y Big_BigInt) Big_BigInt {
	b := bigOf(y)
	bigZeroFault(b)
	return bigKey(new(big.Int).Div(bigOf(x), b))
}

func (x Big_BigInt) M_mod(_ *Task, y Big_BigInt) Big_BigInt {
	b := bigOf(y)
	bigZeroFault(b)
	return bigKey(new(big.Int).Mod(bigOf(x), b))
}

func (x Big_BigInt) M_div_mod(_ *Task, y Big_BigInt) Tuple2[Big_BigInt, Big_BigInt] {
	b := bigOf(y)
	bigZeroFault(b)
	q, r := new(big.Int).DivMod(bigOf(x), b, new(big.Int))
	return Tuple2[Big_BigInt, Big_BigInt]{F0: bigKey(q), F1: bigKey(r)}
}

func (x Big_BigInt) M_exp(_ *Task, y Big_BigInt, m Option[Big_BigInt]) Big_BigInt {
	var mb *big.Int
	if m.Some {
		mb = bigOf(m.V)
	}
	z := new(big.Int).Exp(bigOf(x), bigOf(y), mb)
	if z == nil {
		FaultHere("big.exp: no inverse of the base modulo m for a negative exponent")
	}
	return bigKey(z)
}

func (x Big_BigInt) M_gcd(_ *Task, y Big_BigInt) Big_BigInt {
	a, b := bigOf(x), bigOf(y)
	return bigKey(new(big.Int).GCD(nil, nil, a.Abs(a), b.Abs(b)))
}

func (x Big_BigInt) M_mod_inverse(_ *Task, m Big_BigInt) Option[Big_BigInt] {
	mb := bigOf(m)
	bigZeroFault(mb)
	z := new(big.Int).ModInverse(bigOf(x), mb)
	if z == nil {
		return None[Big_BigInt]()
	}
	return Some(bigKey(z))
}

func (x Big_BigInt) M_sqrt(_ *Task) Big_BigInt {
	a := bigOf(x)
	if a.Sign() < 0 {
		FaultHere("square root of a negative number")
	}
	return bigKey(new(big.Int).Sqrt(a))
}

func (x Big_BigInt) M_neg(_ *Task) Big_BigInt { a := bigOf(x); return bigKey(a.Neg(a)) }

func (x Big_BigInt) M_abs(_ *Task) Big_BigInt { a := bigOf(x); return bigKey(a.Abs(a)) }

func (x Big_BigInt) M_sign(_ *Task) int64 { return int64(bigOf(x).Sign()) }

func (x Big_BigInt) M_cmp(_ *Task, y Big_BigInt) int64 { return int64(bigOf(x).Cmp(bigOf(y))) }

func (x Big_BigInt) M_cmp_abs(_ *Task, y Big_BigInt) int64 {
	return int64(bigOf(x).CmpAbs(bigOf(y)))
}

func bigShiftFault(n int64) {
	if n < 0 {
		FaultHere("negative shift count %d", n)
	}
}

func (x Big_BigInt) M_lsh(_ *Task, n int64) Big_BigInt {
	bigShiftFault(n)
	return bigKey(new(big.Int).Lsh(bigOf(x), uint(n)))
}

func (x Big_BigInt) M_rsh(_ *Task, n int64) Big_BigInt {
	bigShiftFault(n)
	return bigKey(new(big.Int).Rsh(bigOf(x), uint(n)))
}

func (x Big_BigInt) M_and(_ *Task, y Big_BigInt) Big_BigInt {
	return bigKey(new(big.Int).And(bigOf(x), bigOf(y)))
}

func (x Big_BigInt) M_or(_ *Task, y Big_BigInt) Big_BigInt {
	return bigKey(new(big.Int).Or(bigOf(x), bigOf(y)))
}

func (x Big_BigInt) M_xor(_ *Task, y Big_BigInt) Big_BigInt {
	return bigKey(new(big.Int).Xor(bigOf(x), bigOf(y)))
}

func (x Big_BigInt) M_and_not(_ *Task, y Big_BigInt) Big_BigInt {
	return bigKey(new(big.Int).AndNot(bigOf(x), bigOf(y)))
}

func (x Big_BigInt) M_not(_ *Task) Big_BigInt { return bigKey(new(big.Int).Not(bigOf(x))) }

func (x Big_BigInt) M_bit(_ *Task, i int64) int64 {
	if i < 0 {
		FaultHere("negative bit index %d", i)
	}
	return int64(bigOf(x).Bit(int(i)))
}

func (x Big_BigInt) M_bit_len(_ *Task) int64 { return int64(bigOf(x).BitLen()) }

func (x Big_BigInt) M_trailing_zeros(_ *Task) int64 {
	return int64(bigOf(x).TrailingZeroBits())
}

func (x Big_BigInt) M_probably_prime(_ *Task, n int64) bool {
	if n < 0 {
		FaultHere("negative number of Miller-Rabin rounds %d", n)
	}
	return bigOf(x).ProbablyPrime(int(n))
}

func (x Big_BigInt) M_text(_ *Task, base int64) string {
	if base < 2 || base > big.MaxBase {
		FaultHere("invalid base %d", base)
	}
	return bigOf(x).Text(int(base))
}

func (x Big_BigInt) M_bytes(_ *Task) Bytes { return Bytes(bigOf(x).Bytes()) }

func (x Big_BigInt) M_fill_bytes(_ *Task, n int64) Bytes {
	a := bigOf(x)
	if n < 0 || int64((a.BitLen()+7)/8) > n {
		FaultHere("big.fill_bytes: %d bytes do not hold the number", n)
	}
	return Bytes(new(big.Int).Abs(a).FillBytes(make([]byte, n)))
}

func (x Big_BigInt) M_to_int(_ *Task) Option[int64] {
	a := bigOf(x)
	if !a.IsInt64() {
		return None[int64]()
	}
	return Some(a.Int64())
}

func (x Big_BigInt) M_to_float(_ *Task) float64 {
	f, _ := bigOf(x).Float64()
	return f
}

func (x Big_BigInt) M_display(_ *Task) string { return bigOf(x).String() }

func (x Big_BigInt) M_debug(_ *Task) string { return bigOf(x).String() }
