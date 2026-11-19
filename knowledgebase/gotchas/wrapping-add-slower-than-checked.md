# wrapping_add in a hot loop can be slower than a checked +=

Status: Observed (2026-11-19, task #406, linux/amd64 host 1)

## Symptom

Porting Go's pow loop (`ae += xe`, `xe--`, Go's int64 wraps), the
counters were switched from Xo's checked `+=` and `-=` to
`wrapping_add` and `wrapping_sub` to drop the overflow checks. `xo bench
--ab bench/perf/gomath` got slower, not faster: pow 1.026 (1.02 to
1.029), gamma 1.048 (1.047 to 1.049), n=4 each.

## Cause

Not established. `wrapping_add` is the `rt.int_wrap` intrinsic (operation
0, compiler/ssa/call.xo word_method), not a call; the likely cause is
that the checked add's check was already folded (the loop guard bounds
`xe` to +-4096) while `rt.int_wrap` is opaque to the passes that treat a
plain add (range facts, combining, register choice).

## Fix

Keep the checked operators where range facts prove no overflow; measure
with `--ab` before trading `+=` for `wrapping_add` for speed. Use
`wrapping_*` for semantics (Go's wrap on real overflow), not as a speedup.
Next experiment for Lang 1: compare `XO_DUMP_FN=gomath.pow` output of the
two versions to see what `rt.int_wrap` blocks.
