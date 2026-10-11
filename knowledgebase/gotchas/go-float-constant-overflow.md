# Go backend: two float literals multiplied did not compile

Status: Observed (2026-10-11)

Symptom: `let a = 2.5e300 * 1e10` failed `xo build` on the Go backend
with a code generator bug: `float64(2.5e300) * float64(1e10) (constant
2.5e+310 of type float64) overflows float64`. Xo computes `+Inf` at run
time, as the arm64 and LLVM backends do. Found while widening
`progen.Arith` to Float.

Cause: a float literal lowers to `float64(lit)`, a Go typed constant, and
Go folds `+ - *` of two constants at compile time and rejects a result
out of range. `/` (rt.FDiv) and unary `-` (rt.FNeg) were already calls
for the same reason (`0.0 / 0.0`, negative zero).

Fix: a float `+ - *` whose two operands may be Go constants reads the
left one through `rt.FVal` (internal/gen/expr.go, goFloatConst).
Regression: internal/llvm/testdata/golden/float_const_overflow (all
three backends). Any new Go lowering of a float operator on literals
must not leave a Go constant expression that can overflow.
