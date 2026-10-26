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

// Float_fma is `f.fma(y, z)` (decision 0385): math.FMA, exact on every
// architecture (an instruction or Go's software path).
func Float_fma(_ *Task, f, y, z float64) float64 { return math.FMA(f, y, z) }

func Float_mod(_ *Task, f, y float64) float64 { return gmMod(f, y) }

func Float_is_inf(_ *Task, f float64) bool { return gmIsInf(f, 0) }

func Int_leading_zeros(_ *Task, n int64) int64 { return int64(bits.LeadingZeros64(uint64(n))) }

func Int_trailing_zeros(_ *Task, n int64) int64 { return int64(bits.TrailingZeros64(uint64(n))) }

func Int_ones_count(_ *Task, n int64) int64 { return int64(bits.OnesCount64(uint64(n))) }

func Float_to_bits(_ *Task, f float64) int64 { return int64(math.Float64bits(f)) }

func Float_from_bits(_ *Task, b int64) float64 { return math.Float64frombits(uint64(b)) }
