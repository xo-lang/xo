# Moving releases after insert_rc: immortal proof depth and faults

Status: Observed (2026-11-12, Xo Lang 3, decision 0623).

## Symptom

The first versions of the shared release tails pass (compiler/ssa
reltail.xo) built, but the inlined module did not verify:

- `tls13.op: b4: v22 can fault while v1 is held 1 times (no landing
  releases it)`: a release moved past a call in the exit block.
- `xo.json.dec[T]: b485: v27 is held 1 times at ret (leak or
  imbalance)`: a constant Str passed down a ladder of 30 tails.

Without XO_INLINE_STRICT=1 the second one only warned and built the
whole program without inlining, which made the binaries look 10% smaller
than they were (bq 4.43 MB against the real 4.80 MB).

## Cause

- A call with no fault position still faults when its callee can
  (`raises` reads the module's may_fault table); `has_pos` alone misses it.
- insert_rc leaves a value uncounted when imm_at proves it immortal, and
  imm_at follows block parameters back through their edges only 24 deep.
  Each tail adds one parameter hop.

## Fix

- After the first release it moves, an exit may hold only operations
  that cannot fault: no self-faulting op, no call but a runtime `rt.`
  one (may_raise_here).
- An exit whose terminator passes a value that may be immortal (a
  constant, a literal aggregate, a parameter) is left alone
  (may_be_immortal); derived Json decoders pass the field's path segment
  as two Int offsets into one constant (llrt_json_path_push_at).
- Measure sizes with XO_INLINE_STRICT=1, so a build that fell back to no
  inlining fails instead of looking smaller.
