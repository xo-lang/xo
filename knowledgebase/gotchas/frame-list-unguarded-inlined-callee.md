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

## Reduction (2026-11-08)

The golden internal/llvm/testdata/golden/stack_list_tail_callee faults
the same way on 849fcb52f. Three conditions together make the order
that leaves the callee unguarded; a reduction missing any one of them
runs clean, which is why the earlier attempts (and
stack_list_inlined_guard) did not reproduce it:

- the middle function (the challenge policy) has more than
  INLINE_TAIL_MAX (8) instructions of its own, so its tail call of a
  callee over 8 instructions stays a call (find_call's tailbig rule)
  and its own stack_lists builds the literal in the frame;
- the closures it passes sit in an options struct (nested or flat;
  passed as arguments they run clean), since tail_at inlines a tail
  call that passes a closure literal as an argument;
- the functions take their indices breadth first from main (instances
  in the order they are found, not the declaration order), and another
  caller reaches the middle function directly, so it is processed
  before the outer caller while the kept callee comes after it.

Tracing an inlining question: `f.values` keeps dead values after
inline_call, so a call found there may already be inlined; walk the
blocks' instrs instead.
