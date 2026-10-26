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
