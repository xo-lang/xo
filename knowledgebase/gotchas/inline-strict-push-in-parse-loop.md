# The inlined module's verifier rejects a push to a list built in a parse loop with early returns

Status: Observed (2026-10-26, Xo Lang 1)

## Symptom
xobuild of compiler/xo fails with every seed:
`xo: the inlined arm64 module does not verify (1 errors; the first: run_fetch: b61: v245 updates v144, which is neither a reuse result nor provably unique); failing the build (XO_INLINE_STRICT=1)`.
xocheck passed: the checker accepts the code; the error is the SSA verifier after inlining.

## Cause
run_fetch's line loop (many `return None` exits) got a second `var later: List[...]` that it pushed to,
then passed to a function. The uniqueness proof the verifier asks for did not cover that list on the
inlined path. The verifier fails safe (no miscompile reached a binary).

## Fix
Build the list in its own small function (later_files(text)) instead of inside the loop with the early
returns. Report such verifier errors to the lead as a possible gap in the uniqueness proof.
