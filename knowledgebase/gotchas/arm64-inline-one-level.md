# arm64: an inlined helper's own helper calls stay calls, and constants are not folded after inlining

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

selfhost/arm64/inline.xo copies a callee's body as it was before its own
calls were inlined: `fmul(a, b) = freduce(a * b)` is inlined into the
NTT, but the copy's call of freduce stays a call. And the SSA constant
folding runs before inlining, so `rotl(x, n) = rt.int_wrap(5, x, 64 - n)`
inlined with n = 1 keeps `64 - n` as an instruction; the constant 64 is
then hoisted out of the loop and spilled with the other live values.

## Fix

In Xo (what mlkem.xo and sha3.xo do since 2026-10-15): write hot helpers
as leaf functions on rt.* operations directly (no call of another Xo
function), and write a constant shift or rotation amount out
(`rt.int_wrap(5, x, 63)`, not a helper taking the amount). ML-KEM-768 on
arm64 went from 6.41M to 5.10M instructions per CCTV iteration with
these two alone.

In the emitter (not done): inline bottom-up (callees' calls expanded
before a callee is copied), and fold constants again after inlining.
