# arm64: an overflow fault named the operands of `c + x` in swapped order

Status: Observed (2026-10-11)

Symptom: `9223372036854775807 + p0` with p0 = 6 faulted on arm64 with
`integer overflow: 6 + 9223372036854775807 does not fit Int`, where the
Go and LLVM backends print `9223372036854775807 + 6`. Found by the
widened `progen.Arith` (seed 346) once TestArithProperty compared all
of standard error, and reduced by scripts/reduce-diff.py in 87 s.

Cause: `addSub` (internal/arm64/isel.go) swaps a constant left operand
of an add to the right (so it can be an immediate) and then passed the
swapped operands to `xo_fault_overflow`. Only some SSA shapes keep the
constant on the left (the reduced program needs a match on the sum),
so the golden programs never hit it.

Fix: addSub remembers the swap and passes the fault operands in source
order (internal/llvm/testdata/golden/overflow_const_left). When an
instruction selector canonicalizes operands, the fault arguments must
still follow the source.
