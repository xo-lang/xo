package rt

// big.BigFloat (decision 0602): Go's math/big Float as an immutable value.
// Each operation decodes the operands into *big.Float (precision, mode,
// value), calls math/big, and encodes the result with its accuracy. The
// native backends run the port in std/native/big.xo; the bigfloat corpus
// holds both to Go's results.

import (
	"math/big"
)

func floatOf(x Big_BigFloat) *big.Float {
	z := new(big.Float)
	switch x.F_form {
	case 0:
		if x.F_neg {
			z.Neg(z)
		}
	case 2:
		z.SetInf(x.F_neg)
	default:
		z.SetPrec(uint(x.F_prec))
		m := bigOf(x.F_mant)
		z.SetInt(m)
		z.SetMantExp(z, int(x.F_exp)-m.BitLen())
		if x.F_neg {
			z.Neg(z)
		}
	}
	z.SetPrec(uint(x.F_prec))
	z.SetMode(big.RoundingMode(x.F_mode.Tag - 1))
	return z
}

func floatKey(z *big.Float) Big_BigFloat {
	v := Big_BigFloat{
		F_prec: int64(z.Prec()),
		F_mode: Big_RoundingMode{Tag: int(z.Mode()) + 1},
		F_acc:  Big_Accuracy{Tag: int(z.Acc()) + 2},
		F_neg:  z.Signbit(),
		F_mant: bigKey(new(big.Int)),
	}
	switch {
	case z.IsInf():
		v.F_form = 2
	case z.Sign() == 0:
		v.F_form = 0
	default:
		v.F_form = 1
		m := new(big.Float)
		v.F_exp = int64(z.MantExp(m))
		m.SetMantExp(m, int(z.MinPrec()))
		i, _ := m.Int(nil)
		v.F_mant = bigKey(i.Abs(i))
	}
	return v
}

// bigNaN turns math/big's ErrNaN panic into a fault with its text.
func bigNaN() {
	if r := recover(); r != nil {
		if e, ok := r.(big.ErrNaN); ok {
			FaultHere("%s", e.Error())
		}
		panic(r)
	}
}

func bigPrec(prec int64) uint {
	if prec < 0 {
		FaultHere("math/big: negative precision %d", prec)
	}
	if prec > big.MaxPrec {
		return big.MaxPrec
	}
	return uint(prec)
}

// bigZ is new(Float) with x's mode and precision prec (0: math/big's
// default for the operation).
func bigZ(x Big_BigFloat, prec int64) *big.Float {
	z := new(big.Float).SetMode(big.RoundingMode(x.F_mode.Tag - 1))
	if p := bigPrec(prec); p > 0 {
		z.SetPrec(p)
	}
	return z
}

func Big_from_float(_ *Task, x float64, prec int64) Big_BigFloat {
	defer bigNaN()
	if prec == 0 {
		return floatKey(big.NewFloat(x))
	}
	return floatKey(new(big.Float).SetPrec(bigPrec(prec)).SetFloat64(x))
}

func Big_float_from_int(_ *Task, n int64, prec int64) Big_BigFloat {
	return floatKey(new(big.Float).SetPrec(bigPrec(prec)).SetInt64(n))
}

func (x Big_BigInt) M_to_big_float(_ *Task, prec int64) Big_BigFloat {
	return floatKey(new(big.Float).SetPrec(bigPrec(prec)).SetInt(bigOf(x)))
}

func Big_inf(_ *Task, neg bool) Big_BigFloat { return floatKey(new(big.Float).SetInf(neg)) }

func Big_parse_float(_ *Task, s string, base int64, prec int64, mode Big_RoundingMode) Result[Big_BigFloat, Big_BigErr] {
	switch base {
	case 0, 2, 8, 10, 16:
	default:
		FaultHere("invalid number base %d", base)
	}
	f, _, err := big.ParseFloat(textOf(s), int(base), bigPrec(prec), big.RoundingMode(mode.Tag-1))
	if err != nil {
		return Err[Big_BigFloat](Big_BigErr{F_msg: err.Error()})
	}
	return Ok[Big_BigFloat, Big_BigErr](floatKey(f))
}

func (x Big_BigFloat) M_prec(*Task) int64 { return x.F_prec }

func (x Big_BigFloat) M_min_prec(*Task) int64 { return int64(floatOf(x).MinPrec()) }

func (x Big_BigFloat) M_mode(*Task) Big_RoundingMode { return x.F_mode }

func (x Big_BigFloat) M_acc(*Task) Big_Accuracy { return x.F_acc }

func (x Big_BigFloat) M_sign(*Task) int64 { return int64(floatOf(x).Sign()) }

func (x Big_BigFloat) M_signbit(*Task) bool { return x.F_neg }

func (x Big_BigFloat) M_is_inf(*Task) bool { return x.F_form == 2 }

func (x Big_BigFloat) M_is_int(*Task) bool { return floatOf(x).IsInt() }

func (x Big_BigFloat) M_with_prec(_ *Task, prec int64) Big_BigFloat {
	return floatKey(floatOf(x).SetPrec(bigPrec(prec)))
}

func (x Big_BigFloat) M_with_mode(_ *Task, mode Big_RoundingMode) Big_BigFloat {
	v := x
	v.F_mode = mode
	v.F_acc = Big_Accuracy{Tag: 2}
	return v
}

func (x Big_BigFloat) M_add(_ *Task, y Big_BigFloat, prec int64) Big_BigFloat {
	defer bigNaN()
	return floatKey(bigZ(x, prec).Add(floatOf(x), floatOf(y)))
}

func (x Big_BigFloat) M_sub(_ *Task, y Big_BigFloat, prec int64) Big_BigFloat {
	defer bigNaN()
	return floatKey(bigZ(x, prec).Sub(floatOf(x), floatOf(y)))
}

func (x Big_BigFloat) M_mul(_ *Task, y Big_BigFloat, prec int64) Big_BigFloat {
	defer bigNaN()
	return floatKey(bigZ(x, prec).Mul(floatOf(x), floatOf(y)))
}

func (x Big_BigFloat) M_quo(_ *Task, y Big_BigFloat, prec int64) Big_BigFloat {
	defer bigNaN()
	return floatKey(bigZ(x, prec).Quo(floatOf(x), floatOf(y)))
}

func (x Big_BigFloat) M_sqrt(_ *Task, prec int64) Big_BigFloat {
	defer bigNaN()
	return floatKey(bigZ(x, prec).Sqrt(floatOf(x)))
}

func (x Big_BigFloat) M_neg(*Task) Big_BigFloat {
	v := x
	v.F_neg = !x.F_neg
	v.F_acc = Big_Accuracy{Tag: 2}
	return v
}

func (x Big_BigFloat) M_abs(*Task) Big_BigFloat {
	v := x
	v.F_neg = false
	v.F_acc = Big_Accuracy{Tag: 2}
	return v
}

func (x Big_BigFloat) M_cmp(_ *Task, y Big_BigFloat) int64 {
	return int64(floatOf(x).Cmp(floatOf(y)))
}

func (x Big_BigFloat) M_mul_pow2(_ *Task, e int64) Big_BigFloat {
	if x.F_form != 1 {
		return x
	}
	z := floatOf(x)
	return floatKey(z.SetMantExp(z, int(e)))
}

func (x Big_BigFloat) M_mant_exp(*Task) Tuple2[Big_BigFloat, int64] {
	if x.F_form != 1 {
		return Tuple2[Big_BigFloat, int64]{F0: x, F1: 0}
	}
	v := x
	v.F_exp = 0
	return Tuple2[Big_BigFloat, int64]{F0: v, F1: x.F_exp}
}

func (x Big_BigFloat) M_to_int(*Task) Tuple2[int64, Big_Accuracy] {
	i, a := floatOf(x).Int64()
	return Tuple2[int64, Big_Accuracy]{F0: i, F1: Big_Accuracy{Tag: int(a) + 2}}
}

func (x Big_BigFloat) M_to_float(*Task) Tuple2[float64, Big_Accuracy] {
	f, a := floatOf(x).Float64()
	return Tuple2[float64, Big_Accuracy]{F0: f, F1: Big_Accuracy{Tag: int(a) + 2}}
}

func (x Big_BigFloat) M_to_big(*Task) Tuple2[Option[Big_BigInt], Big_Accuracy] {
	i, a := floatOf(x).Int(nil)
	acc := Big_Accuracy{Tag: int(a) + 2}
	if i == nil {
		return Tuple2[Option[Big_BigInt], Big_Accuracy]{F0: None[Big_BigInt](), F1: acc}
	}
	return Tuple2[Option[Big_BigInt], Big_Accuracy]{F0: Some(bigKey(i)), F1: acc}
}

func (x Big_BigFloat) M_text(_ *Task, format int64, prec int64) string {
	return floatOf(x).Text(byte(format), int(prec))
}

func (x Big_BigFloat) M_display(*Task) string { return floatOf(x).String() }

func (x Big_BigFloat) M_debug(*Task) string { return floatOf(x).String() }
