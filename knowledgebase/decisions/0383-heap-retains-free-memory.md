# 0383: The heap hands back none of the memory the build frees, so xo building itself peaks at 10.4 GB with 0.2 GB live

Status: Proposed (Xo Lang 3, 2026-10-26, for the owner and the lead; nothing built). Follows the
cold peak study the lead asked for after tokens as offsets proved not to be the peak lever.

## Context

Measured 2026-10-26 on the current main xo (/tmp/ss/xosa/bin/xo, a native build), cold builds, one
profiled run each, a private XO_BACKEND_CACHE_DIR.

Footprint against live memory (`--phase-times` heap and peak, and XO_PROF=heap with
XO_PROF_HEAP_AT=PHASE, which is what is alive after the phase):

| program | phase | footprint (heap / peak) | live |
|---|---|---|---|
| llvm-fib | past fe.build | 392 / 393 MB | 262 MB |
| llvm-fib | past emit | 428 / 440 MB | 66 MB |
| llvm-fib | past assemble | 414 / 461 MB (RSS 488 MB) | 55 MB (past undef) |
| xo itself | past fe.build | 3778 / 3779 MB | 2.71 GB |
| xo itself | past emit | 10385 / 11079 MB | 209 MB |
| xo itself | past hollow | 11006 / 11707 MB | n/a |
| xo itself | at exit | RSS 8.6 GB, peak footprint 10.4 GB | |

(xo itself, past emit: the heap profile lost 869 K samples, so its split by function is approximate;
the total is not.) The peak of a cold build of xo is 50 times its live set. This is the memory the
reaper stopped jobs for and the 37 GB incident of 2026-10-22.

A scratch hook (in a scratch worktree, never pushed: heapos.xo walked every heap record's free
lists, the spare magazines and the central magazines, and read each carved block's class from its
8 byte prefix) reports who holds the free memory after a phase:

- Whose lists: past emit on xo itself 7,320 MiB in 1,830 chunks, free: per-thread lists 31 MiB,
  spare magazines 61 MiB, the central lists 5,330 MiB (98%). llvm-fib past emit: 100 chunks,
  per-thread 21 MiB, spare 23 MiB, central 286 MiB (87% of 330). Past fe.build the central lists
  hold only 18 to 20 MiB, so the memory is not the front end freed at the strip: it is the
  emit phase's own churn (xo allocates 33 GB over its emit phase, through 9 heap records).
- Which classes (xo itself past emit, MiB carved, free, central):

  | classes | carved | free | central |
  |---|---|---|---|
  | 2 to 50 (32 to 800 B) | 1,405 | 823 | about 790 |
  | 65 (2 KiB) | 692 | 672 | 671 |
  | 66 (4 KiB) | 223 | 194 | 193 |
  | 67 (8 KiB) | 496 | 454 | 454 |
  | 68 (16 KiB) | 471 | 391 | 389 |
  | 69 (32 KiB) | 849 | 752 | 749 |
  | 70 (64 KiB) | 1,119 | 980 | 973 |
  | 71 (128 KiB) | 727 | 561 | 550 |
  | 72 (256 KiB) | 903 | 631 | 608 |

  Of 6,885 MiB carved, 5,458 MiB are free; the classes of 2 KiB and over hold 4,635 MiB of it
  (85%). llvm-fib past emit shows the same shape in 360 MiB: classes 65 to 67 (2 to 8 KiB) carved
  189 MiB and free 184 MiB, classes 68 to 72 carved 79 MiB and free 46 MiB.
- Which chunks: the memory is not in chunks that could be given back. Past emit, xo itself has 952
  chunks of 1,830 at least 90% free (3,781 MiB of the free memory in them) but 0 or 1 chunks wholly
  free, because a few live blocks (and the unused tail a replaced chunk leaves) pin each chunk; llvm-fib
  has 32 of 90 chunks at least 90% free (124 MiB) and 0 fully free.

The allocator (std/native/llrt/alloc.xo, heapos.xo): 72 size classes (16 B steps to 1 KiB, powers of
two 2 KiB to 256 KiB), a free list per class per thread's heap record, magazines of
MAG_BYTES = 64 KiB (at least 8 blocks) moved to central lists when a list passes its cap, blocks
carved from 4 MiB chunks, chunks never unmapped, a free block never coalesced and never moved to
another class. A thread whose class is empty takes a magazine of the same class from the central
list, else carves fresh from its chunk.

Go's allocator, for comparison (runtime/mheap.go, mgcscavenge.go): objects live in spans (runs of
8 KiB pages) of one size class; when the sweep finds every object of a span free, the span goes back
to the page heap, where adjacent free pages coalesce, and any class can take them; a background
scavenger returns free pages to the OS (madvise MADV_DONTNEED on Linux, MADV_FREE on macOS, paced
to hold RSS near the heap goal plus 10%, and GOMEMLIMIT's limit). Xo has neither of its two pieces:
no span is ever wholly returned (a class's blocks of all ages share a chunk, with no per chunk or
per span live count), and nothing is returned to the OS.

## Decision (proposed, in the order to build)

0. First a timeline, not a build: sample the per class carve and free bytes (the scratch hook) every
   100 ms through xo's own cold build, to learn whether the classes emit carves late are the ones it
   freed early. This decides how much of (a) below is reachable (it is the difference between the
   sum of the classes' peaks, what the heap holds today, and the peak of their sum, the most (a) can
   bring it to). It needs no allocator change.
1. (c) Scavenge: when a magazine of a class of 32 KiB or over is pushed to the central lists (cold
   memory: no thread's list holds it), return the pages inside each of its blocks that the block
   fully covers, except the page holding the block's link word (madvise MADV_FREE, or
   MADV_FREE_REUSABLE on Darwin; the kernel gives back zero pages that refault on reuse, which the
   allocator allows because a block's contents are undefined). Expected on xo itself past emit:
   the free blocks of classes 69 to 72 total 2,924 MiB; the page granule is 16 KiB, so per block
   a 32 KiB block gives 0 to 50%, 64 KiB 50 to 75%, 128 KiB 75 to 88% and 256 KiB 88 to 94%:
   1.5 to 2.2 GB of the 8.6 GB RSS (17 to 26%) with no change to the lists, the classes or any
   address. The cost is a madvise call per 32 KiB or larger block pushed, and a page fault per
   reused page (16 KiB), against the allocator's cost of 33 GB allocated in 17 s today.
2. (a) Cross class reuse from the central lists: a class whose own central list and spare are
   empty takes a magazine of a larger class (the smallest class with one) and cuts each block into
   blocks of its own size (each cut block needs its 8 byte prefix, written into the old block's
   payload; the leftover tail is cut into the smaller classes, or dropped for sizes under 16 B),
   before it carves. Blocks only get smaller, never merge back, which keeps the allocator's
   no coalescing rule. Upper bound on xo itself: the 5.3 GB on the central lists would be
   reachable by any class, so the heap could shrink toward the peak of the classes' sum; the
   timeline of step 0 gives the real bound. A risk: a split block can never serve a big class again
   (a long build ends with every class small), so (a) is kept to the classes of 2 KiB and over
   at first, where one 256 KiB block cuts into 128 blocks of 2 KiB.
3. (b) Whole chunk release alone recovers almost nothing, not the 2.3 to 3.0 GB the 90% figure
   suggests: 0 or 1 chunks of 1,830 are wholly free past emit and past assemble. What it takes:
   a per chunk live count (an increment on every alloc and decrement on every free, on the hot
   path; or a sweep that sums the free bytes per chunk from the central lists as the scan did,
   about 10 s on xo itself with a linear lookup and about 100 ms with 4 MiB aligned chunks), plus
   removing a released chunk's blocks from the lists. Even then the 90% chunks are held by their live
   blocks. It becomes useful only with (a) (cutting the blocks out of a nearly free chunk first,
   so the chunk drains) and is not worth building alone.

Rank by expected peak saving on xo itself: (a) first in potential (up to the 5.3 GB on the central
lists, unknown until step 0) but the least certain and the largest change; (c) first to build
(1.5 to 2.2 GB, certain, a few dozen lines, nothing else moves); (b) last (0 alone).

## Gate (for every step)

The fixed point (scripts/bootstrap-xo.sh s2 and s3 identical), a self build's peak RSS and peak
memory footprint before and after (`/usr/bin/time -l xo build ./compiler/xo`, a cold private cache,
heavy.sh bulk), `xo bench --ab` on the alloc and concurrency benches (bench/perf alloc, maps, text,
http and the scheduler's) in a quiet window, because the madvise of (c) and the splitting of (a) sit
on the central path every thread's refill takes, and XO_HEAPSTATS' free list report unchanged in
shape. The allocator's no allocation rule (check_no_alloc) holds for alloc.xo and heapos.xo's
changes.

## Consequences

xo's own peak falls from 10.4 GB toward the live set plus the classes' working set, which is what
bounds how many builds the Mac runs at once (decision heavy.sh HEAVY_MAX_GB). Memory a process frees
returns to the OS, as Go's does, with the cost of refaulting it on reuse.

## Alternatives

- Per thread caches that take from the central lists of any class: the same as (a) with the
  splitting done at the central lists' lock; costs a lock hold per cut.
- Coalescing free blocks of a class into the next class up (the buddy system): needs the per chunk
  or per span structure the allocator does not have; (a) covers the direction that matters (large
  freed, small wanted).
- Fewer allocations in emit (arenas per function): shrinks the churn itself (33 GB in 17 s) but is a
  compiler change per pass, not a heap change; kept as the next item after step 0 shows which classes
  the churn is in.
- Lowering MAG_BYTES so fewer blocks sit in magazines: leaves the memory in lists of the wrong class.
