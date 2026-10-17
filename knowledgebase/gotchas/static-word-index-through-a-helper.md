# rt.static_word's index must be a literal at the call, not a helper's parameter

Status: Observed (2026-10-14, work item B27)

## Symptom

A runtime module that wraps `rt.static_word` in a small helper
(`fn word(k: Int) -> Int { rt.mem_load(rt.static_word(k), 0, 8) }`,
called as `word(W_VEC)`) builds on LLVM but fails on the arm64 backend:

    std/native/llrt/tzhost.xo:199:38: error XO1201: not available without
    a garbage collector (`--no-gc`) yet: `rt.static_word` of a word that is
    not a constant 0 to 63 (support is planned)

(The `--no-gc` wording is the generic text of XO1201, not the cause.)

## Cause

The SSA builder checks `rt.static_word`'s argument where it is called
(compiler/ssa call.xo raw_intrinsic): it must be a constant there.
Inside the helper `k` is a parameter. LLVM's build happened to pass
because the check ran on an inlined copy; the arm64 build checks the
helper itself.

## Fix

Write `rt.static_word(W_NAME)` with the constant at every use (as
testworld.xo and logstate.xo do), or make the helper take the address
(`rt.static_word(W_X)` computed by the caller).
