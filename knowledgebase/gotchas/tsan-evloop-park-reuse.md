# ThreadSanitizer: evloop's poller park CAS against a plain read in run_item

Status: Observed (2026-10-16), not explained yet

## Symptom

TestRuntimeEventLoop built with `--race` (every function marked,
524edb0c) fails about one run in three under load (four copies at once
reproduce it on the first round, any of XO_THREADS 1, 3, 8):
"Atomic write of size 8 ... by thread T1: x.poll.poll_main" (a
compare and swap of a park's lock word: signal's slot_cas(p, 0, 0, 1)
for a fired timer) against "Previous read of size 8 ... x.pool.run_item"
(an `ldr` of word 0 of a record run_item's inlined fiber code reads, at
the same address).

## Cause (suspected)

The address holds a park (in a context record) when the poller signals
it and a fiber or task record when run_item reads it: Xo's own heap
reuses the freed block, and ThreadSanitizer sees no happens-before edge
through a free and a reallocation that it does not intercept (it
models malloc and free, not std/native's alloc.xo free lists). Not yet
proven: the read may also be a real plain read of a live park.

## Fix

Until explained, internal/llvm runRuntimeXo builds evloop without the
marks (raceUnchecked). Next: annotate the heap's free and reuse for
ThreadSanitizer (__tsan_release on a free, __tsan_acquire on the
reallocation of the block, through sanhook.xo as the fiber annotations
are), then rebuild evloop marked; or log the block's owner at both
accesses to tell the two causes apart.
