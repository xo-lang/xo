# ThreadSanitizer reports a race on the free after a release decrement and an acquire fence

Status: Observed (2026-10-08, std/llrt/testdata/rc_threads.c, decision 0058)

Symptom: with the shared release written as Rust's Arc does it
(`__atomic_fetch_add(p, 1, __ATOMIC_RELEASE)`, then
`__atomic_thread_fence(__ATOMIC_ACQUIRE)` in the thread that saw -1),
TSan reported a data race between another thread's atomic decrement and
the plain write `xo_free` makes into the freed block (its free list
link), in the box chain test.

Cause: ThreadSanitizer does not model standalone fences, so it never
sees the acquire that orders the other threads' releases before the
free. The code was correct; the report was a false positive.

Fix: the last release uses one read-modify-write with acquire and
release order (`__ATOMIC_ACQ_REL` in C, `atomicrmw ... acq_rel` in
LLVM). On arm64 both forms are a single `ldaddal`, so nothing is lost,
and TSan understands it. A test that reads a count a generated code path
reads plainly (the inline `reuse` check) must read it with a relaxed
atomic load in C, or TSan reports that read too.
