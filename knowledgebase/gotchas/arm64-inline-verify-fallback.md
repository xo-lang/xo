# An arm64 inliner change that fails verification silently drops all inlining

Status: Observed (2026-10-15).

## Symptom

A change to compiler/arm64 inline.xo made programs much slower on
arm64 (chacha 2500M to 5199M instructions) with calls where helpers
had been inlined, and every test still passed.

## Cause

compiler/xo main.xo uses the inlined module only when it verifies
(arm64 inline_step_jobs: `!inline_module(m) || verify ok`); otherwise
it quietly emits the module without inlining. The change produced SSA
the verifier rejects (a counted value held on one edge of a merge and
not on another), so the whole program lost its inlining.

## Fix

When changing the inliner, check that the module verifies: temporarily
make inline_step_jobs fault with the verifier's first error, and count
the `bl` of a known hot function before and after. Copy a callee into
the caller's block only when its values are all scalars (counted values
must pass through the continuation's parameter).

Since 2026-10-15 the fallback is loud: a build says it on standard
error (the first verification error names the function), and fails
with XO_INLINE_STRICT=1, which internal/xotool sets for the Go tests
(TestInlineFallbackLoud breaks the module on purpose with
XO_INLINE_BREAK=1). Since 2026-10-21 a build whose per-function cache is
used verifies the inlined module after dead function removal, with the
cache's function keys (computed once for both; compiler/xo main.xo
defer_inline_verify); XO_INLINE_BREAK=1 keeps the old order (its break
may land in a function the removal drops) and XO_INLINE_BREAK=late
breaks the module after the removal, the test of the deferred path.

Observed again (2026-10-17, Xo Lang 2): an SSA reference counting change
read as a 16% cut of the TLS client's code in `xo asm`, which was the
fallback (that build without inlining); measured against main it was
+0.04%, and the compiler's own build had fallen back (its first error:
ssa.Builder.cleanup_all) while the fixed point still passed. Since then
the message starts with `warning:`, and scripts/bootstrap-xo.sh builds s2
and s3 with XO_INLINE_STRICT=1, so a self build that falls back fails
the fixed point. Measure RC, borrowing, and inlining changes with
XO_INLINE_STRICT=1 set.

Observed again (2026-10-18, xo-4, Go compiler audit): the new SSA cse
pass, run inside inline_module after inlining, removed an operation that
had a landing pad (its block branches on the landing), so x509's module
failed the reference count check and was built without inlining.
tlssrv.flush measured 3,667 to 353 instructions, a figure from the
uninlined build; with the fix (cse keeps a value with a landing pad) it
was 3,043. A pass added to inline_module is subject to the same
fallback: measure it with XO_INLINE_STRICT=1.

Observed again (2026-10-24, Xo Lang 1): making a constant read's list
immortal in rc.xo (imm_at const_list: no retain or release) left a
borrowed element of that list "not alive" in the verifier (alive() of a
projection asks its parent's count, which is never counted now), so
every program that read a constant in an inlined function was built
without inlining. The A/B, built in a scratch script that kept only
`| tail -1` of the build, read DS-2's constant loops as 3% and 7% more
instructions; the warning line never showed. Fix: verify.xo alive()
treats an immortal owned value as alive. Build every A/B binary with
XO_INLINE_STRICT=1, and never drop a measurement build's standard error.
