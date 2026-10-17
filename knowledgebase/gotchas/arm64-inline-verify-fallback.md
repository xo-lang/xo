# An arm64 inliner change that fails verification silently drops all inlining

Status: Observed (2026-10-15).

## Symptom

A change to selfhost/arm64 inline.xo made programs much slower on
arm64 (chacha 2500M to 5199M instructions) with calls where helpers
had been inlined, and every test still passed.

## Cause

selfhost/xo main.xo uses the inlined module only when it verifies
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
XO_INLINE_BREAK=1).

Observed again (2026-10-17, Xo Lang 2): an SSA reference counting change
read as a 16% cut of the TLS client's code in `xo asm`, which was the
fallback (that build without inlining); measured against main it was
+0.04%, and the compiler's own build had fallen back (its first error:
ssa.Builder.cleanup_all) while the fixed point still passed. Since then
the message starts with `warning:`, and scripts/bootstrap-xo.sh builds s2
and s3 with XO_INLINE_STRICT=1, so a self build that falls back fails
the fixed point. Measure RC, borrowing, and inlining changes with
XO_INLINE_STRICT=1 set.
