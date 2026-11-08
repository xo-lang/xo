# A frame list meets an inlined callee's unguarded keep

Status: Observed (2026-11-07)

## Symptom

The arm64 build of azure's storage/azblob (and storage/azdatalake)
faulted after decision 0554 landed:
`fault: stack_lists: core_with_token: v183 escape after the later passes (decision 0554)`
at compiler/arm64/inline.xo, inlining through the cache.

## Cause

0554's rule decides a function's small literals on its own post-inline
SSA, and relies on every callee guarding its List parameters
(unstack_params: `rt.rl_unstack` before a use that keeps one). That
guard is added in the callee's own inline_one, but a caller can inline
a callee whose inline_one has not run yet: its body then keeps the
parameter unguarded. core_with_token inlined
new_storage_challenge_policy (with its frame list `[audience]`) and
runtime.new_bearer_token_policy, whose struct took `[audience]`
directly: a real use after the frame (the verifier was right).
Turning off any single later pass (hoist_fields, fold_tuples, cse,
take_in_place) did not help, which pointed away from the passes.

## Fix

inline_call takes `guarded_callee(m, callee)`: the body with
unstack_params applied first (idempotent: param_keeps treats an
existing `rt.rl_unstack` as the guard). The verifier
(stack_list_violations) accepts what a guarded inlined body brings:
`rt.rl_unstack` of the frame list itself, and Retain, Release and Drop
of it (a frame list's count is STACK: they change nothing); and its
fault now names the use (`v231 struct uses v184`).
