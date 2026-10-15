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
