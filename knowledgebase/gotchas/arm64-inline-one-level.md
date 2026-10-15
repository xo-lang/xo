# arm64: a helper over the inliner's block limit stays a call, and constants were not folded after inlining

Status: Observed (2026-10-15, xo-fa, the ML-KEM speed work: std/native/llrt
mlkem.xo and sha3.xo on the arm64 backend).

## Symptom

A hot loop calls small helpers that are all within the inliner's budget,
yet the arm64 code still has `bl` to some of them, and rotations by a
constant amount compile to a load from a stack slot, a `sub`, and a
`ror` by register:

    ldr x16, [sp, #1168]
    sub x9, x16, #1
    ror x9, x11, x9

`sample` showed mlkem.freduce and mlkem.fsub as their own frames inside
mlkem.ntt (a quarter of the samples); the LLVM build had no such calls.

## Cause

Two, one of them first misread (corrected 2026-10-15, the same day):

- Not a one-level copy. inline.xo copies a callee's body with its calls,
  and inline_module's find_call then rescans the caller and inlines the
  copied calls too, so nesting works. The calls that stayed were of
  helpers over the cost limits: a helper of checked arithmetic has a
  block per overflow branch, and the cost of a helper counts each
  inlinable callee's blocks plus one, so `fsub` over reduce_once, add,
  sub and mul came to 13 blocks against INLINE_MAX_BLOCKS 12 and was not
  inlinable at all.
- The SSA constant folding ran only before inlining, so
  `rotl(x, n) = rt.int_wrap(5, x, 64 - n)` inlined with n = 1 kept
  `64 - n` as an instruction; the constant 64 was then hoisted out of
  the loop and spilled with the other live values.

## Fix

In the emitter (2026-10-15): inline.xo fold_consts folds the Int
operations on constants again after a function's calls are inlined
(checked ones only when they do not overflow, shifts by 0 to 62,
rt.int_wrap's wrapping add, subtract and multiply; values with a landing
check and sized integers left alone). The rotation repro now compiles to
`asr x9, x12, #61` where it was `sub x9, x8, #3; asr x9, x12, x9`.

Not done, measured and rejected: counting a one-block scalar callee's
copy as no block (so fsub fits the 12 blocks). It made more helpers
inlinable everywhere; the whole bench/perf set on arm64 had 41 workloads
slower beyond their spread (alloc small 1.078, text lines 1.039, json and
digest 1.004 to 1.013), the same longer-blocks cost as
bench/results/arm64-inline-2026-10-15.md's first attempt, and it changed
nothing in ML-KEM after the rewrite below. A first version of it also
let stdio.out_slow (a cold path split off on purpose, one call site,
under INLINE_FEW_BLOCKS once its helpers counted as no blocks) be copied
into stdio.out, and small writes took 31% more instructions.

In Xo (what mlkem.xo and sha3.xo do since 2026-10-15), still the way for
a hot loop: write hot helpers as leaf functions on rt.* operations
directly (no call of another Xo function, no checked arithmetic), and
write a constant shift or rotation amount out (`rt.int_wrap(5, x, 63)`).
ML-KEM-768 on arm64 went from 6.41M to 5.10M instructions per CCTV
iteration with these two alone.
