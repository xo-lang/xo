package rt

// big.BigRat (decision 0608): Go's math/big Rat as an immutable value.
// Each operation decodes the operands into *big.Rat, calls math/big, and
// encodes the result in lowest terms. The native backends run the port
// in std/native/big.xo; the bigrat corpus holds both to Go's results.

import (
	"math/big"
)

func ratOf(x Big_BigRat) *big.Rat { return new(big.Rat).SetFrac(bigOf(x.F_a), bigOf(x.F_b)) }

func ratKey(z *big.Rat) Big_BigRat {
	return Big_BigRat{F_a: bigKey(new(big.Int).Set(z.Num())), F_b: bigKey(new(big.Int).Set(z.Denom()))}
}

func Big_rat(_ *Task, a Big_BigInt, b Big_BigInt) Big_BigRat {
	d := bigOf(b)
	bigZeroFault(d)
	return ratKey(new(big.Rat).SetFrac(bigOf(a), d))
}

func Big_rat_from_int(_ *Task, n int64) Big_BigRat { return ratKey(new(big.Rat).SetInt64(n)) }

func (x Big_BigInt) M_to_rat(*Task) Big_BigRat { return ratKey(new(big.Rat).SetInt(bigOf(x))) }

func Big_rat_from_float(_ *Task, f float64) Option[Big_BigRat] {
	z := new(big.Rat).SetFloat64(f)
	if z == nil {
		return None[Big_BigRat]()
	}
	return Some(ratKey(z))
}

func Big_parse_rat(_ *Task, s string) Result[Big_BigRat, Big_BigErr] {
	z := new(big.Rat)
	if err := z.UnmarshalText([]byte(s)); err != nil {
		return Err[Big_BigRat](Big_BigErr{F_msg: err.Error()})
	}
	return Ok[Big_BigRat, Big_BigErr](ratKey(z))
}

func (x Big_BigRat) M_add(_ *Task, y Big_BigRat) Big_BigRat {
	return ratKey(new(big.Rat).Add(ratOf(x), ratOf(y)))
}

func (x Big_BigRat) M_sub(_ *Task, y Big_BigRat) Big_BigRat {
	return ratKey(new(big.Rat).Sub(ratOf(x), ratOf(y)))
}

func (x Big_BigRat) M_mul(_ *Task, y Big_BigRat) Big_BigRat {
	return ratKey(new(big.Rat).Mul(ratOf(x), ratOf(y)))
}

func (x Big_BigRat) M_quo(_ *Task, y Big_BigRat) Big_BigRat {
	r := ratOf(y)
	if r.Sign() == 0 {
		FaultHere("division by zero")
	}
	return ratKey(new(big.Rat).Quo(ratOf(x), r))
}

func (x Big_BigRat) M_neg(*Task) Big_BigRat { return ratKey(new(big.Rat).Neg(ratOf(x))) }

func (x Big_BigRat) M_abs(*Task) Big_BigRat { return ratKey(new(big.Rat).Abs(ratOf(x))) }

func (x Big_BigRat) M_inv(*Task) Big_BigRat {
	r := ratOf(x)
	if r.Sign() == 0 {
		FaultHere("division by zero")
	}
	return ratKey(new(big.Rat).Inv(r))
}

func (x Big_BigRat) M_num(*Task) Big_BigInt { return x.F_a }

func (x Big_BigRat) M_denom(*Task) Big_BigInt { return x.F_b }

func (x Big_BigRat) M_sign(*Task) int64 { return int64(ratOf(x).Sign()) }

func (x Big_BigRat) M_cmp(_ *Task, y Big_BigRat) int64 { return int64(ratOf(x).Cmp(ratOf(y))) }

func (x Big_BigRat) M_is_int(*Task) bool { return ratOf(x).IsInt() }

func (x Big_BigRat) M_to_float(*Task) Tuple2[float64, bool] {
	f, ex := ratOf(x).Float64()
	return Tuple2[float64, bool]{F0: f, F1: ex}
}

func (x Big_BigRat) M_float_prec(*Task) Tuple2[int64, bool] {
	n, ex := ratOf(x).FloatPrec()
	return Tuple2[int64, bool]{F0: int64(n), F1: ex}
}

func (x Big_BigRat) M_text(*Task) string { return ratOf(x).String() }

func (x Big_BigRat) M_rat_text(*Task) string { return ratOf(x).RatString() }

func (x Big_BigRat) M_float_text(_ *Task, prec int64) string { return ratOf(x).FloatString(int(prec)) }

func (x Big_BigRat) M_to_big_float(_ *Task, prec int64, mode Big_RoundingMode) Big_BigFloat {
	return floatKey(new(big.Float).SetPrec(bigPrec(prec)).SetMode(big.RoundingMode(mode.Tag - 1)).SetRat(ratOf(x)))
}

func (x Big_BigFloat) M_to_rat(*Task) Tuple2[Option[Big_BigRat], Big_Accuracy] {
	r, a := floatOf(x).Rat(nil)
	acc := Big_Accuracy{Tag: int(a) + 2}
	if r == nil {
		return Tuple2[Option[Big_BigRat], Big_Accuracy]{F0: None[Big_BigRat](), F1: acc}
	}
	return Tuple2[Option[Big_BigRat], Big_Accuracy]{F0: Some(ratKey(r)), F1: acc}
}

func (x Big_BigRat) M_display(*Task) string { return ratOf(x).String() }

func (x Big_BigRat) M_debug(*Task) string { return ratOf(x).String() }
