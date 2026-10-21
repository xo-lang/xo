# The literal -9223372036854775808 is XO1201 on the native backends

Status: Observed (2026-10-20, std/native/binary_test.xo, decision 0279); fixed 2026-10-20

## Symptom

A test table generated from Go holding `math.MinInt64` as the literal
`-9223372036854775808` fails to build natively:
`error XO1201: not available without a garbage collector (--no-gc) yet:
this Int literal (support is planned)`. The Go backend accepts it.

## Cause

The literal is `-` applied to `9223372036854775808`, which does not fit
in an Int; the Go backend folds the two into Go's constant, the native
SSA builder does not.

## Fix

Write `(-9223372036854775807 - 1)`; a generator that prints Int values
for Xo should print MinInt64 that way.

Fixed (2026-10-20, poll item): the SSA builder folds `-` into the
literal 9223372036854775808 (compiler/ssa expr.xo UnaryExpr), so
`-9223372036854775808` is Int's minimum on every backend, in an
expression, an Int64, and a pattern (golden
internal/llvm/testdata/golden/int_min_literal). The workaround above is
no longer needed.
