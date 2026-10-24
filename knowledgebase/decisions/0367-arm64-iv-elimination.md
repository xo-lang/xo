# 0367: arm64 loops drop the index beside a pointer induction variable, with post-indexed loads (extends 0054)

Status: Proposed (Xo Lang 1, 2026-10-24; for the owner via the lead, with 0366; nothing built)

## Context

The native-vs-LLVM sweep of bench/perf on main (2026-10-24, notes
sweep-native-vs-llvm-2026-10-24.txt) has callbacks ifor at 1.351 of
LLVM's instructions and for at 1.445 (two_for 1.466). A repro of their
loops alone (`for u in xs { if u.a == want { return true } }`, 1,000
structs walked 20,000 times) measures ifor 1.348 and for 1.584; for's
call of obj.str_eq for every name of the needle's length is fixed
separately (calls.xo str_eq_const, 2026-10-24: for 0.781 of its
instructions); what is left in both is the loop.

Native's ifor loop is 7 instructions an element:

    ldr  x9, [x10, #0x20]   ; u.a
    cmp  x9, x12
    b.eq found
    add  x11, x11, #1       ; i
    add  x10, x10, #0x18    ; the element pointer (0054's pointer IV)
    cmp  x11, x23           ; i < len
    b.lt loop

LLVM's is 5:

    ldr  x7, [x6], #0x18    ; u.a, post-indexed: the pointer steps
    cmp  x7, x5
    b.eq found
    subs x4, x4, #1         ; a down counter
    b.ne loop

0054 built the pointer induction variable but keeps the index for the
bound test and left post-indexed loads unbuilt ("not applicable to the
bench loops: the pointer is used after the load, by the swap"); these
search loops use the pointer only for the load.

## Decision (proposed)

In compiler/arm64 lsr.xo, after plan_lsr's pointer parameters:

1. Index elimination: when a loop's index i is used only by its own
   step (i + 1), the bound test (i < n, n loop invariant), and the
   element addresses that a pointer induction variable already replaces,
   i becomes a down counter (n - i on entry, `subs c, c, #1` and `b.ne`
   at the latch, one instruction where add, cmp were two), or, when the
   counter would need a register the loop does not have, the bound test
   compares the pointer with an end pointer computed before the loop.
   An index used anywhere else (a store of i, a call, a fault's index
   operand) keeps it.
2. Post-indexed loads: a load through the pointer induction variable at
   offset k, when the pointer is not read after it in the iteration, is
   `ldr x, [p], #step` with the pointer started k bytes on (the step add
   is folded into the load), as LLVM does. The stores keep their form
   (decision 0054's and today's store placement findings: M-series lost
   wall on every RSA store change, so stores are not touched here).
3. Under XO_PASSES=-lsr neither applies (the build stays correct); x86
   (whose addressing has no post-increment) gets step 1 only.

## Measurement plan

1. Hand-edit the native assembly of the ifor repro to LLVM's 5
   instruction shape and time both in a quiet window (`xo bench --ab`):
   instructions and wall. Stop and record here if the wall does not
   improve (ifor's wall is already 1.03 of LLVM's).
2. Built: every arm64 golden (XO_INLINE_STRICT=1), progen's loop
   programs, the codegen patterns (a pattern for the post-indexed load
   and the absent index compare, checked to fail on the xo before), the
   fixed point, the Docker amd64 sample (step 1 there), and quiet
   windows on bench/perf callbacks, json, text, strsplit and rsa
   against the parent.

## Consequences

- Fewer instructions in every search loop over a list of structs (the
  callbacks rows, and loops like it in std: find, contains, any written
  as a for).
- A down counter changes register use in the loop (one live value
  replaced by another); no new register.

## Alternatives

- Leave the index: ifor stays at 1.35 of LLVM's instructions, though its
  wall is already about LLVM's.
- Index elimination without post-indexing: 6 instructions, the simpler
  half; it can land first if post-indexing measures badly.
