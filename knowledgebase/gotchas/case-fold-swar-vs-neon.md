# The ASCII case fold is already SWAR, and clang widens the same loop

Observed 2026-10-25, working item "Str.to_lower and to_upper on ASCII-heavy
text, still about 1.75 of LLVM's instructions" (the case rows of
`bench/perf/unicode`, which the lead's plan proposed to fix with a
word-at-a-time ASCII path in std).

## Symptom

The plan described the ASCII fold as a per rune loop to replace with eight
byte SWAR. It is not a per rune loop, and the SWAR it asked for is there
already. The 1.755 (lower) and 1.697 (upper) of LLVM's instructions on
native arm64 against 0.82 to 0.85 on the mixed rows comes from the other
direction: clang widens that same loop to 16 byte NEON, so the same source
costs 1.31 instructions a byte on arm64 against about 0.4 on LLVM.

## Cause

`std/native/llrt/obj.xo`: `rs_append_bytes` with a case mode hands an
ASCII run to `fold_copy_lo` or `fold_copy_up`, which fold eight bytes a
step with two broadcast adds, two masks, a shift and an add. An ASCII byte
never reaches `case_append`'s decode and table path. A source-level unroll
to two words an iteration takes the arm64 row to 0.861 but costs LLVM
1.017 (two loops to widen, plus a second setup of the constants per
call), and a one loop version with the second word under a test compiles
to 29 instructions per 16 bytes against the base loop's 30.

## Fix

Before writing "the fast path is missing", read the fold in `obj.xo` and
measure the two backends' instructions on the same row
(`bench/perf/unicode/measure.sh XO OUT arm64 llvm`): the numbers say which
side of the comparison the gap is on. `bench/results/case-swar-2026-10-25.md`
has the rows of both forms. The remaining lever is vector width, which a
std change cannot reach: it needs an arm64 NEON fold intrinsic in the
shape of `rt.bytes_high16` (`compiler/arm64/calls.xo`).
