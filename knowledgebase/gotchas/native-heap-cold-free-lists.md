# Native heap: allocation is a cache miss when a size class's free list is cold

Status: Observed (2026-10-13, xo1 compiling selfhost/xo; xo1-speed work)

Symptom: a native program that allocates and frees many small blocks
in waves (xo1: millions of MIR instructions per function, freed when
the function is done) spends a large share of its time in `xo_alloc`
itself: 26.6 percent of xo1's samples, 31.4 percent after the bit set
passes were removed. `perf annotate` puts nearly all of it on the
instruction after the load of the free block's link. The process also
keeps its high water mark: 23.9 GB of resident memory for 5.6 billion
allocations with nothing live at exit.

Cause: std/llrt's heap pops blocks from per size class free lists
(LIFO). After a wave of frees the list holds blocks scattered over the
heap, so each pop loads a line no cache holds, one at a time (the next
link is in the block just popped). Blocks on a class's list are never
returned to the system nor used for another class, so a list that grew
through several classes leaves its old blocks behind in each.

Fix: allocate less (bench/results/xo1-speed-2026-10-13.md: an empty
list literal no longer allocates, decision 0198; temporary lists, per
block bit sets, and per load strings removed from the emitter), and
the allocator prefetches the new head's link on each pop. To see where
a program's heap went, run it with `XO_HEAPSTATS=1` (the blocks on
each class's list at exit); `XO_HEAP_MID_MAX=1024` sends blocks over
1 KiB to malloc (xo1: 8.8 GB to 7.9 GB, same time).
