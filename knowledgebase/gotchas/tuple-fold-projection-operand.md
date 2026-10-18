# A tuple fold moves a projection's count onto no leaf

Status: Observed (2026-10-18, branch xo4-flush, ssa.fold_tuple_fields)

Symptom: with fold_tuple_fields in arm64 inline_module, the bootstrap's
stage s2 failed: "the inlined arm64 module does not verify
(cached_macho_objects: b61: release of v188, which is not held)". The
standalone compiler/arm64 driver compiled compiler/xo without a word: it
falls back to the module without inlining when the inlined one does not
verify, and says nothing (only `xo build` reports it, and fails under
XO_INLINE_STRICT=1).

Cause: v188 was `call list.index`, a projection (rc.xo kind Proj: its
count starts at 0); the `retain v188` before `tuple v183, v188` made the
tuple's leaf its owner, and the releases of `field 1` of the tuple
released that leaf. Folding the field reads into v188 put those
releases on v188's own leaf, which the retain had not counted for.

Fix: fold only a tuple whose counted operands are owned roots
(passes.xo tuple_operand_owned: not a borrowed parameter, constant,
payload, capture, unbox, field or list element). To check a compiler
change that only the inliner exercises, verify the inlined module and
fail on an error (a driver built with a fault on mi.verify_all()), not
by the driver's output.
