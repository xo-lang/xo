# An unlocked check of a static word races its locked writer under TSan

Status: Observed, fixed (2026-10-30, Xo lang 5; the lead's 21:57 main
check at 0a50ca1c7, intermittent: earlier checks passed).

Symptom: internal/llvm TestGoldenTasks/http_h2c (ThreadSanitizer, LLVM)
reports a data race on a global `.xo.static` word: a write in
`xo_x_heap_big_release` (heapos.release's cache insert, keep, inlined)
against an earlier read in `x.heapos.large_room` (take, inlined).

Cause: heapos.xo take() checked the large block cache's table word
(rt.static_word(W_CACHE)) with a plain `rt.mem_load` before taking the
cache's lock, as a fast way out when no table exists yet; keep() stores
the table's address there, under the lock, the first time a block is
cached. The read is outside the lock, so nothing orders it with the
store: a real data race in the memory model (benign in practice, since
take rereads the word under the lock), and TSan reports it whenever a
first release on one thread meets a large allocation on another. Decision
0425's page blocks (4c397e82b) sent more blocks through these paths;
the unlocked read itself dates from 87e1e5aac (2026-10-14).

Fix: the pair is atomic: `rt.slot_load(w, 0, ACQUIRE)` for the unlocked
check and `rt.slot_store(w, 0, t, RELEASE)` for the store (TSan sees
LLVM's atomics). Any lock free fast path that reads a word a locked path
writes needs the same: an atomic load and an atomic store, not plain
mem_load and mem_store.
