# The rc verifier models borrowed projections on its own

Status: Observed (2026-11-03, Xo Lang 1)

## Symptom

A change to which projections rc borrows (compiler/ssa/borrow.xo) builds
programs that run correctly, but every golden built with
`XO_INLINE_STRICT=1` fails, and xo cannot build itself: `the inlined arm64
module does not verify (13 errors; the first: arm64.G.x86_emit_inst: b235:
v2940 uses v993.1, which is not alive)`.

## Cause

The verifier (compiler/ssa/verify.xo `verify_rc`, run on the inlined
module) does not read rc's borrow decisions (`bproj` is empty in
`rc_checker`). It treats every projection that holds no count as alive
while its syntactic parent's leaf is (`alive`). When borrow.xo let a
projection of `b.segs` outlive `b.pos = ...` (decision 0186's owner
chain), the parent's root `b` is consumed by the setfield; the leaf lives
on in the setfield's value, which the verifier did not know.

## Fix

Change the verifier's model in the same commit as the borrow rule
(`carried_alive`: a parent leaf moved into a setfield of another field is
followed there). Gate a borrow change on a self build or on the goldens
with `XO_INLINE_STRICT=1` (TestGoldenRun sets it), not only on rc dumps
and standalone runs: the program built without strict inlining falls
back silently and passes.
