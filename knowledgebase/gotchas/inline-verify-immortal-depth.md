# A long chain of field updates loses its immortal fields in the inline verify

Status: Observed (2026-11-15).

## Symptom

A function that builds a struct with an inlined constructor, updates
twenty or more of its fields, then updates a field conditionally and
joins paths, builds without inlining: `xo: warning: built without
inlining (... edge to bN holds {vM.22:1 ... vM.26:1}, another
predecessor {...})`, and fails with XO_INLINE_STRICT=1. The listed
fields are immortal values (here `b""` from the constructor's literal).
Lang 3 met it in std's tlsio.client_with (`if ech_list.is_some() {
cf.legacy_suites = [] }` after tls13.conf). 0.20.0 to 0.51.1 have it;
the output was always correct, only slower.

## Cause

compiler/ssa/rc.xo imm_at, which the reference counting and the
verifier share, gave up past 24 steps, and every field update on the
way back to the literal was a step. Before inlining, the walk ends at
the constructor's call, whose summary says the field is immortal; after
it, the walk runs through the inlined literal's parameter and every
update, past 24. The verifier then counted those fields on the path
that consumed the struct and not on the other.

## Fix

Since 2026-11-15 an update of another field spends no depth: the chain
is straight (an SSA value comes before its uses), so only block
parameters and descents into a literal count. Golden
internal/llvm/testdata/golden/inline_immortal_depth.
