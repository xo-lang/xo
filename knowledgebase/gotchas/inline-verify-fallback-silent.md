# A failed inline verification silently builds without inlining

Status: Observed (2026-10-17, Xo Lang 2).

## Symptom

A change to the SSA reference counting looked like it cut the TLS
client's arm64 code by 16% (422,715 to 354,311 instructions in `xo asm`).
The same change measured again against main was +0.04%. Building the
compiler with it printed, once, among the build output: `the inlined arm64
module does not verify (10 errors; the first: ssa.Builder.cleanup_all:
...), so it is built without inlining`.

## Cause

When the module inlined by selfhost/arm64 inline.xo fails the SSA
verifier, a normal build (and `xo asm`) falls back to the module before
inlining and goes on; only `XO_INLINE_STRICT=1`, which the Go tests set,
turns it into an error. The 16% was the cost of inlining itself (the
TLS client without inlining), not the change. The fixed point passed
too: a compiler built without inlining still rebuilds itself to a fixed
point.

## Fix

- Measure a change to RC, borrowing, or inlining with
  `XO_INLINE_STRICT=1` in the environment, so a verification failure
  stops the build instead of changing the code being measured.
- Grep a self build's output for `does not verify` (the fixed point does
  not catch it).
