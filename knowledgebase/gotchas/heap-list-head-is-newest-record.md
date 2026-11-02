# The heap record list's head is the newest record, not the main thread's

Observed 2026-11-02 (tier 38, linux llvm step on the second host).

**Symptom:** ThreadSanitizer data races in `x.heapos.chunk` (read) against
`xo_x_heap_thread` (write) on `.xo.static`, failing TestRuntimeRealtime,
TestRuntimeThreads, TestRuntimeTasks, TestRuntimeAlloc and
TestGoldenTasks/{unix_kill,websocket_tls_deadline} in internal/llvm.

**Cause:** `heapos.reserve` (467c08692, XO_THP=dense) found the main thread's
heap record by comparing it with `static_word(W_HEAPS)`. `thread_heap` pushes
each new record at that word, so the word holds the newest record: a thread
whose record was the newest took the main thread's branch (its first chunk
advised MADV_HUGEPAGE whole), and the plain read raced with the push.

**Fix:** the main thread's record is the list's last one, `H_NEXT == 0`
(thread_heap links every other record before it); the test reads only the
record itself. The main thread's record also gains: it was the head only until the
first thread_heap push, so after that the old check stopped advising its later first
chunks too (Xo Lang 2's review).

**Avoid:** read `W_HEAPS` only under `hlock` or at startup (one thread); to
identify a record use its own fields.

**Second race, same day:** `thp_mode` (called on every chunk once dense was the
default) reads the huge page mode through the list head with no lock, while
`thread_heap` stored the head with a plain store: TSan again, and on arm64 no
ordering for the record's fields. The head is now stored with release and read
by `thp_mode` with acquire.
