# The SIGURG handler's test must not read the thread local table (LLVM)

Status: Observed (2026-11-17, task #306)

## Symptom

On main after 1ccfd45e1 (task #213), `TestPreempt/llvm` and
`TestAsyncPreempt/llvm/preempt_regs` and `preempt_alloc` failed: "a
spinning fiber was not preempted", 0 preemptions in three runs. The
native backends' runs were unaffected by the change.

## Cause

1ccfd45e1 added `obj.defer_busy()` to `pool.async_held`, the test the
SIGURG handler (`pool.async_signal`) runs before it preempts. defer_busy
read `rt.tlocal_get(TLS_HEAP)`. In an LLVM build, a runtime function that
reaches the thread local table holds the fiber's no-preemption count
(F_NOPRE) while it runs (compiler/llvm apreempt.xo, decision 0318 stage
4), and only the handler entries themselves (`apre_exempt`) are spared.
So async_held and defer_busy became guarded: while the handler ran its
test, the fiber's F_NOPRE was 1, the test read it, and every preemption
was declined. A variant whose defer_busy loads were never true still
failed; only removing the call passed.

## Fix

The handler reads the thread local words itself (it is exempt) and passes
them in: `pool.async_held(w, f, th)` and `obj.defer_busy_in(th)` read no
thread local word. Any new term in async_held follows the same rule: no
`rt.tlocal_*` call in it or anything it calls, or LLVM builds never
preempt.
