package rt

import (
	"math"
	"math/bits"
)

// std's math (decision 0287): Go's portable algorithms with every product
// rounded (gomath_gen.go, generated from Go's math package with each
// float64 product wrapped in float64(...), which forbids fused
// multiply-add), so the Go backend gives the native backends' bits on
// every architecture; package math would fuse on arm64 and run assembly
// on amd64.

func Float_pow(_ *Task, f, y float64) float64 { return gmPow(f, y) }

func Float_exp(_ *Task, f float64) float64 { return gmExp(f) }

func Float_exp2(_ *Task, f float64) float64 { return gmExp2(f) }

func Float_expm1(_ *Task, f float64) float64 { return gmExpm1(f) }

func Float_log(_ *Task, f float64) float64 { return gmLog(f) }

func Float_log2(_ *Task, f float64) float64 { return gmLog2(f) }

func Float_log10(_ *Task, f float64) float64 { return gmLog10(f) }

func Float_log1p(_ *Task, f float64) float64 { return gmLog1p(f) }

func Float_sin(_ *Task, f float64) float64 { return gmSin(f) }

func Float_cos(_ *Task, f float64) float64 { return gmCos(f) }

func Float_tan(_ *Task, f float64) float64 { return gmTan(f) }

func Float_asin(_ *Task, f float64) float64 { return gmAsin(f) }

func Float_acos(_ *Task, f float64) float64 { return gmAcos(f) }

func Float_atan(_ *Task, f float64) float64 { return gmAtan(f) }

func Float_sinh(_ *Task, f float64) float64 { return gmSinh(f) }

func Float_cosh(_ *Task, f float64) float64 { return gmCosh(f) }

func Float_tanh(_ *Task, f float64) float64 { return gmTanh(f) }

func Float_cbrt(_ *Task, f float64) float64 { return gmCbrt(f) }

func Float_atan2(_ *Task, f, x float64) float64 { return gmAtan2(f, x) }

func Float_hypot(_ *Task, f, y float64) float64 { return gmHypot(f, y) }

func Float_copysign(_ *Task, f, sign float64) float64 { return gmCopysign(f, sign) }

func Float_frexp(_ *Task, f float64) Tuple2[float64, int64] {
	frac, exp := gmFrexp(f)
	return Tuple2[float64, int64]{frac, int64(exp)}
}

// Float_ldexp is `f.ldexp(exp)`: an exp beyond Go's int (none on 64 bit
// targets) is the same Int.
func Float_ldexp(_ *Task, f float64, exp int64) float64 { return gmLdexp(f, int(exp)) }

// Float_fma is `f.fma(y, z)` (decision 0385): math.FMA, exact on every
// architecture (an instruction or Go's software path).
func Float_fma(_ *Task, f, y, z float64) float64 { return math.FMA(f, y, z) }

func Float_mod(_ *Task, f, y float64) float64 { return gmMod(f, y) }

// Go's math beyond the first set (task #383), each from gomath_gen.go.
func Float_signbit(_ *Task, f float64) bool { return gmSignbit(f) }

func Float_modf(_ *Task, f float64) Tuple2[float64, float64] {
	i, frac := gmModf(f)
	return Tuple2[float64, float64]{i, frac}
}

func Float_round_to_even(_ *Task, f float64) float64 { return gmRoundToEven(f) }

func Float_next_after(_ *Task, f, y float64) float64 { return gmNextafter(f, y) }

func Float_remainder(_ *Task, f, y float64) float64 { return gmRemainder(f, y) }

func Float_acosh(_ *Task, f float64) float64 { return gmAcosh(f) }

func Float_asinh(_ *Task, f float64) float64 { return gmAsinh(f) }

func Float_atanh(_ *Task, f float64) float64 { return gmAtanh(f) }

func Float_erf(_ *Task, f float64) float64 { return gmErf(f) }

func Float_erfc(_ *Task, f float64) float64 { return gmErfc(f) }

func Float_erfinv(_ *Task, f float64) float64 { return gmErfinv(f) }

func Float_erfcinv(_ *Task, f float64) float64 { return gmErfcinv(f) }

func Float_gamma(_ *Task, f float64) float64 { return gmGamma(f) }

func Float_lgamma(_ *Task, f float64) Tuple2[float64, int64] {
	l, s := gmLgamma(f)
	return Tuple2[float64, int64]{l, int64(s)}
}

func Float_is_inf(_ *Task, f float64) bool { return gmIsInf(f, 0) }

func Int_leading_zeros(_ *Task, n int64) int64 { return int64(bits.LeadingZeros64(uint64(n))) }

func Int_trailing_zeros(_ *Task, n int64) int64 { return int64(bits.TrailingZeros64(uint64(n))) }

func Int_ones_count(_ *Task, n int64) int64 { return int64(bits.OnesCount64(uint64(n))) }

func Float_to_bits(_ *Task, f float64) int64 { return int64(math.Float64bits(f)) }

func Float_from_bits(_ *Task, b int64) float64 { return math.Float64frombits(uint64(b)) }
