# 0257: loop invariant fields of list elements and proven indexes

Status: Accepted (Xo Lang 2, 2026-10-17; the lead's queue: nbody's field
level alias reasoning on the arm64 and amd64 backends).

## Context

nbody was the largest compute gap to LLVM on arm64. Its inner loop
(`advance`) read `bs[i].x`, `y`, `z` and `mass` in every iteration and
checked `i` against the length twice in every iteration, though neither
changes in the loop: 53 instructions an iteration against LLVM's 38.

Both are invisible to the instruction selector. Its memo of facts and
loads lives in a block and the blocks it dominates, and does not enter a
loop through the header. Its loop invariant code motion
(hoist_invariants) moves only arithmetic, not loads.

The SSA has the information: a list is a value, and `bs[j].vx += d` is
`list.set(L, j, setfield(3, list.index(L, j), ...))`, a new list equal to
L except in field 3 of one element.

## Decision

1. compiler/arm64 fieldlicm.xo, on each function after inlining (inside
   inline_module, so the module verifier checks the result with the
   inlined code under XO_INLINE_STRICT=1):
   - A value `field k(list.index(L, idx))` in a natural loop moves to the
     loop's preheader when all of these hold:
     - its type is Int, Float or Bool, so it is read without reference
       counting;
     - idx is defined outside the loop;
     - L reaches back to a parameter of the loop's header (or to a value
       defined outside the loop) only through list.set calls that store
       `setfield(k2, list.index(X, j), y)` with k2 != k to the same X and
       j, through reuse, and through parameters of other blocks of the
       loop whose incoming values agree (field_root);
     - every back edge's value for that parameter does the same;
     - the index is proven in range at the preheader (point 2).
   - The moved list.index has no bounds check. This holds whatever j is:
     when j == idx, the stored element keeps field k.
   - At most 8 fields leave one loop. Inner loops go first, so a field can
     then leave the outer loop too.
2. An index is proven in range in block B when both hold:
   - a branch on `idx < list.len(L2)` took its true edge into a block
     that has only that edge and dominates B, and L2 always has the
     list's length (len_root: through list.set, reuse and agreeing block
     parameters);
   - idx is not negative (nonneg): a constant at least 0, a length, a
     block parameter whose incoming values are, or a sum of such that
     cannot wrap. A sum cannot wrap when the add is checked, or when it
     adds 0 or 1 to a with `a < n` known where it runs, n not negative.
3. The instruction selector's bounds_check uses the same proof (G.proof,
   built once a function) for list.index, list.set and list.take. A
   proven check emits nothing. list.set keeps its position in the SSA
   because the reference counting verifier reads it (keeps_operands), so
   the proof is applied at emission rather than by clearing it.
4. Not done: no fused multiply-add. Float is IEEE 754 with no contraction
   allowed (spec 3), and LLVM's build does not fuse either.

## Consequences

- nbody on arm64 (the Mac, instructions retired): 3,586M to 3,240M
  (-9.7%), against LLVM's 2,701M: from 1.33 to 1.20 times. Its inner loop
  is 45 instructions an iteration: no `i` checks and four fewer loads.
  The rest of bench/perf/xo and the backend suite are within 0.4% (noise
  on reruns); push -0.4% to -0.8%. Binaries are the same size, and the
  self build's time is the same within the loaded machine's noise.
- The golden list_field_licm covers these cases: fields read past sets of
  other fields with i == j; the same field set (not hoisted); a whole
  element set; a list replaced and grown in the loop; an inner loop that
  runs no times; an unproven index that faults inside the loop. Output is
  identical on main's arm64, LLVM and this change.
- What is left of nbody's gap to LLVM: LLVM pairs loads and stores (ldp,
  stp, q registers) and does x/y in 2-wide SIMD (fsub.2d, faddp). The
  arm64 backend has neither: the next steps in
  bench/results/amd64-vs-llvm-2026-10-17.md.

## Alternatives

- Keep the facts across loop headers in the instruction selector's memo.
  It knows registers, not which field a store writes, so it cannot see
  that a store to bs[j].vx leaves bs[i].x. The SSA can.
- Alias analysis on the selector's loads and stores by base register and
  offset. bs[i] and bs[j] are different registers that can hold the same
  address, and a store and a load at different offsets of the same
  element type cannot overlap only when the offsets are known relative to
  one element. That is the field level reasoning again, done later and
  with less information.
- Hoist only the fields and keep the checks. That was measured: nbody
  -1.9%, because the `i` and `j` checks moved to the stores.
