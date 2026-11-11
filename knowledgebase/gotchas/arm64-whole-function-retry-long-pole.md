# A whole-function retry doubles the self build's long pole

Status: Observed (2026-11-10, Xo Lang 1, ADR 0564 step 3)

## Symptom

0564 step 3 (native sret for wide results) made `xo bench --ab -- stage2
self` 13.6% slower in wall time but only 4.8% in cpu. Emit grew from about
11.2 s to 17.0 s while every other phase moved by under 0.2 s.

## Cause

G.function (compiler/arm64/isel.xo) lowers a function again with a smaller
cap when the first frame passes the ldr reach (the 700-word outgoing area
for wide results, then 256). The two largest functions always took that
path: compiler/xo's build_native (95,954 SSA values) and lower (42,204).
build_native runs alone in its emit part and is the end of the parallel
emit, so lowering and allocating it twice (10.2 s to 19.6 s) added its
whole cost to the wall. A per-function cost that averages out over cpu
lands at full size on wall when the function is the long pole.

## Fix

A function over SRET_WIDE_VALUES (20,000) SSA values starts with the small
cap, which is what its retry produced (isel.xo SRET_WIDE_VALUES). Before adding
any "try, then lower again" path, check what it costs on the largest
functions: `XO_EMIT_JOBS` per-part times or a probe printing the function
name and value count on each retry. Every function in the retry list
pays twice.

The self build alone is not enough to clear a codegen change for build
time: with this fix the self build measured 1.003 on host 1, and
aiplatform/v1's cold test build still emitted 11 times slower (493.7 s
against 43.8 s, 2026-11-10), so step 3 was reverted again. A change to
how wide values are lowered also times a cold build of a wide-struct
SDK module (xo-lang/gcp aiplatform/v1) before it lands.
