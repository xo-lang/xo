# xo's own heap by size class, per phase (2026-10-26, Xo Lang 3)

llvm-fib built by the main xo, a scratch hook (not committed: /tmp/ss/scratch-heapphase.diff, a
`debug.mem_classes()` printed after every phase) walking every heap's free lists
(heapos.nfree), and XO_PROF=heap XO_PROF_HEAP_AT=PHASE for the live heap by allocation site.
Before the front end was freed in default builds (dd9e4a954, gotchas/front-end-kept-under-auto-check-cache.md).

## Cold build (mapped at the end of the phase; chunks are the 4 MiB carve chunks, the rest is big
blocks over 256 KiB; free lists in MB by size)

| after phase | chunks MB | mapped MB | free <=256 B | free 272 B..8 KiB | free >=16 KiB |
|---|---|---|---|---|---|
| check | 152 | 178 | 0 | 0 | 1 |
| fe.build | 324 | 392 | 2 | 18 | 1 |
| cleanup | 364 | 432 | 10 | 25 | 18 |
| insert_rc (peak 436) | 368 | 436 | 13 | 31 | 22 |
| inline (the front end goes) | 376 | 383 | 9 | 238 | 33 |
| hollow | 388 | 443 | 23 | 249 | 46 |
| link | 388 | 392 | 32 | 255 | 74 |

By class at the end (MB): 2 KiB 51, 4 KiB 64, 8 KiB 84, 16 KiB 17, 32 KiB 15, 64 KiB 12, 128 KiB 12,
256 KiB 17. At exit 361 of the 388 MB of chunks are on free lists; the live peak was about 370 MB.

## Edit rebuild (chunks 232 MB, mapped 292 MB peak)

Free lists at the insert_rc peak 58 MB; after the front end goes 205 MB (134 MB in 2 to 8 KiB blocks:
4 KiB 56, 8 KiB 54); live heap 171 MB (tz), 141 MB (insert_rc), 12 MB (assume).

## What it says

1. Chunks are never unmapped, free blocks never coalesce and no page of a free block is given back
   (no madvise or munmap of a free block anywhere in alloc.xo or heapos.xo): the 250 MB of 2 to 8 KiB
   blocks the checker leaves behind serve only requests of their own class. Later phases (hollow's
   text, link's objects) take big blocks (over 256 KiB, mapped fresh) and larger classes, so the
   mapped size grows 383 to 443 MB while 300 MB sit on free lists.
2. The classes from 2 KiB are powers of two (65 to 72): a 2.1 KiB request takes 4 KiB. The tables and
   lists of the checker fall in these classes.
3. Ranked fixes (none built): MADV_FREE of the interior pages of free blocks of 16 KiB and over (RSS
   only, the 61 MB of the >=16 KiB classes, a fault on reuse); coalescing free blocks into larger ones
   (needs address order, a change to the free path of every allocation: --ab on every alloc bench);
   four classes per doubling from 2 KiB (internal waste 25% to about 12%); a region for the front end
   (dropped whole). Not decided: see the lead's order.

## Size class rounding of the live heap (2026-10-26, edit rebuild of llvm-fib, after dd9e4a954)

A scratch hook in `xo prof` (not committed) rounds the sampled live blocks of an
XO_PROF_HEAP_AT snapshot to the allocator's classes (16 byte steps to 1 KiB, powers of two from 2 KiB
to 256 KiB, pages above) with each block's 8 byte prefix; MB, live requested -> as classes:

| at the end of | <=1 KiB | 2 KiB | 4 KiB | 8 KiB | 16 KiB | 32 KiB | 64 KiB | 128 KiB | 256 KiB | big | total |
|---|---|---|---|---|---|---|---|---|---|---|---|
| check | 18 -> 19 | 3 -> 4 | 34 -> 44 | 29 -> 43 | 1 -> 1 | 3 -> 5 | 5 -> 6 | 4 -> 5 | 2 -> 3 | 4 -> 4 | 106 -> 139 |
| fe.build | 22 -> 24 | 3 -> 5 | 37 -> 48 | 39 -> 58 | 7 -> 9 | 8 -> 10 | 9 -> 11 | 6 -> 7 | 6 -> 8 | 27 -> 27 | 166 -> 212 |
| ssa (the peak) | 29 -> 31 | 5 -> 7 | 45 -> 59 | 38 -> 56 | 3 -> 4 | 6 -> 8 | 8 -> 10 | 7 -> 9 | 6 -> 8 | 25 -> 26 | 174 -> 221 |
| cleanup (the front end gone) | 7 -> 9 | 1 -> 1 | 1 -> 1 | 1 -> 1 | 2 -> 3 | 2 -> 3 | 3 -> 4 | 1 -> 2 | 2 -> 3 | 3 -> 3 | 27 -> 34 |

The rounding is 46 MB at the ssa peak (26% of the requested bytes), 32 MB of it in the 4 KiB and
8 KiB classes (the checker's tables and lists: a request between 2 KiB and 4 KiB takes 4 KiB). At
that point the chunks are 212 MB (class bytes of the non big blocks about 195 MB, the free lists 4
MB, the bump chunks' unused tails the rest, about 13 MB) and the mapped size 271 MB (the big blocks
and the runtime's own mappings about 59 MB). Four classes per doubling from 2 KiB would cut the
rounding to about half (an estimate: a request is on average 12% over its class instead of 26%),
about 25 MB of the 272 MB peak (9%), at the cost of a larger class table and more free lists.
