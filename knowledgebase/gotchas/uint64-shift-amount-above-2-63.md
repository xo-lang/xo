# A UInt64 shift amount of 2^63 or more: the Go backend shifts, the native backends fault

Status: Observed (2026-10-13, internal/llvm TestIntsDiff, decision
0195); not fixed (it predates the port: the C path behaves as the Xo
one).

Symptom: `x << y` and `x >> y` with `x: UInt64` and `y: UInt64` at or
above 2^63 give 0 on the Go backend, and on the native backends fault
with `shift amount -1 out of range for UInt64` (the amount printed as
an Int).

Cause: std/rt's `Shl` and `Shr` (arith.go) test `n < 0 ||
int64(n) >= bits`; for an unsigned n at or above 2^63, `int64(n)` is
negative, so neither test fires and Go's shift gives 0. The native
lowering (internal/ssa build_int.go `sizedOp`, selfhost/ssa show.xo
`sized_op`, as xo_int_op did) tests the amount's word as an Int, and
the message prints it as one: the shift amount has been converted to the
shifted value's type (`shiftCount`), so an Int -1 and a UInt64 2^64 - 1
are the same word there.

Fix: TestIntsDiff leaves these amounts out (`if y <=
9223372036854775807`). Next: decide the semantics (the spec says an
out of range amount faults, so the Go backend is wrong): test
`uint64(n) >= bits` for unsigned N in std/rt's Shl and Shr, and pass the
amount's own type to `sizedOp` (from the expression, before
`shiftCount`) so the native message prints a UInt64 amount unsigned;
then drop the guard from TestIntsDiff.
