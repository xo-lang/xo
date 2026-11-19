# math/big SetMantExp takes the argument's precision

Status: Observed (2026-11-19, task #434)

Symptom: a Go backend BigFloat add or sub at precision 0 came out with 53
bits (or the other operand's) where Go gives the larger operand
precision: TestStdCorpora/go/bigfloat "Float add is Go's" failed after
308558569.

Cause: `new(big.Float).SetPrec(p).SetMantExp(big.NewFloat(0.5), e)` is
53 bits, not p: SetMantExp sets z from its mant argument with Set, and
the argument's precision wins when the stand in is built
this way (checked with Go 1.26: Prec() is 53).

Fix: call SetPrec (and SetMode) after SetMantExp, as floatOf in
std/rt/bigfloat.go does.
