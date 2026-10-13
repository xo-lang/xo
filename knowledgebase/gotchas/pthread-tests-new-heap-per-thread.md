# C runtime tests: every new pthread gets a new heap and a new chunk

Status: Observed (2026-10-13, writing std/llrt/testdata/alloc_threads.c)

Symptom: a stress test whose threads (fresh pthreads each round)
allocate, with one thread freeing everything, carved 224 chunks in 32
rounds, failing a bound meant to show that the central lists bring
freed blocks back (decision 0118), on both the Xo and the C heap.

Cause: a thread's heap comes from the spare list only when a task
worker that exited gave its heap back; a plain pthread that runs
runtime code gets a new heap (calloc) on its first allocation, which
carves a new 4 MiB chunk, and its heap is never reused. Seven
allocating threads a round is seven chunks a round, whatever the
central lists do.

Fix: in a test of chunk reuse, allocate on a thread that lives
through every round (the main thread, as tasks.c's magazines test
does) and free on the new threads; alloc_threads.c's waves test does
that and bounds the chunks at 4 over 30 rounds.
