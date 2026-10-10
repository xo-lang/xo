# The native allocator kept freed blocks on the freeing thread

Status: Observed (2026-10-10, decision 0118)

Symptom: after the scope pruning fix, a native HTTPS server under a
connection per request still grew about 700 bytes per connection (223 MB
peak RSS after 20 s); `bench/perf/conc` `spawn` peaked at 45 MB for a
60 ms run. `leaks` and `XO_LEAKCHECK=1` were clean, and `heap` showed
only the runtime's 4 MiB chunks (`xo_refill`).

Cause: `xo_free` puts a block on the calling thread's free list,
whatever thread allocated it. When one thread allocates and others free
(an accept loop and its connection tasks, a loop that spawns tasks that
end on workers), the allocating thread's lists stay empty and it carves
new chunks, while the freeing threads' lists only grow.

Fix: with threads, free lists move between heaps as magazines through a
central list per size class (`free_mt`, `alloc_mt`): 28 MB after the
same 20 s. Reusing blocks that other cores wrote is slower than carving
fresh ones in spawn heavy microbenchmarks (15 to 24 percent): compare
peak RSS (`/usr/bin/time -l`) as well as time when changing the
allocator, since the leak check counts blocks, not memory held.
