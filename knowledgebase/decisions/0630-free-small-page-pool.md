# 0630: Free small pages: whole free pages of the small classes leave their free lists for a pool every class carves from

Status: Proposed (Xo Lang 1, 2026-11-13); a runtime change in every native program, so the owner's yes comes before any code

## Context

After decisions 0628 and 0629, xo's own peak is mostly memory the heap holds free, not memory
in use. In the gcp aiplatform/apiv1 test build (caches off, Mac), 315 MB is live when assembly
starts, while the heap has 2.07 GB mapped and a 2.44 GB peak footprint. XO_HEAPSTATS at exit
shows 1.56 GB on the free lists, spread over about 70 size classes.

Decision 0383 already returns part of this. Its (c) scavenges the central magazines of the 32 KiB
and larger classes (madvise, MADV_FREE_REUSABLE on darwin). Its (a) cuts central magazines of the
classes below 32 KiB into blocks of a smaller class before a thread maps a new chunk. What neither
reaches is the free blocks of the small classes (below PAGE_CLASS 77): they stay resident in their
4 MiB chunks, and they serve only their own class. A freed block's list link and its 8 byte prefix
sit in the block itself, so a page holding free blocks cannot simply be advised: the kernel may
zero it, and the lists would break. That is why (c) keeps the link's page.

## The census (2026-11-13, a scratch hook, never pushed)

At each top level phase label (debug.mem_label), the hook walked every small class's free blocks
(each thread record's lists and magazines, and the central magazines). It added each block's bytes,
prefix included, to the pages it covers, at 16 KiB (darwin's page) and at 4 KiB (Linux's), and
counted the pages that are free whole. A walk at a label inside a phase ("emit start 0") raced
emit's workers and stopped the build, so the census runs only between phases. The same applies to
the HTTP server under load, which faulted until its census waited for the burst to end.

| program | footprint | when | mapped | free small | 16 KiB pages touched / free whole | 4 KiB pages touched / free whole |
|---|---|---|---|---|---|---|
| aiplatform/apiv1 test build (xo) | 2.44 GB | past emit | 2,088 MB | 1,108 MB | 78,953 / 13,474 (210 MB) | 315,699 / 104,888 (409 MB) |
| | | past assemble | 2,087 MB | 1,115 MB | 80,432 / 8,582 (134 MB) | 321,626 / 91,624 (357 MB) |
| the self build of compiler/xo | 2.91 GB | past emit | 3,551 MB | 1,773 MB | 128,676 / 32,927 (514 MB) | 514,497 / 173,224 (676 MB) |
| | | past assemble | 3,550 MB | 1,751 MB | 129,116 / 26,226 (409 MB) | 516,267 / 145,126 (566 MB) |
| bench/perf/alloc mixed (ring live) | 43 MB | in the run | 10 MB | 3 MB | 221 / 219 (3 MB) | 884 / 882 (3 MB) |
| bench/perf/alloc grow, strings, maps, big1, bign, tasks | 36 to 41 MB | after the run | 2 to 14 MB | 0 MB | | |
| bench/perf/http CRUD server, 10 s at 32 connections, then idle | 32 MB | idle after | 47 MB | 3 MB | 233 / 109 (1 MB) | 894 / 703 (2 MB) |
| the same at 256 connections | 43 MB | idle after | 69 MB | 7 MB | 488 / 256 (4 MB) | 1,883 / 1,527 (5 MB) |

So the free small memory is interleaved with live blocks. In aiplatform, 81% of the 16 KiB pages
holding free small blocks also hold a live one. The pages free whole are what a page granular
return can reach:
- xo's own builds: 5 to 9% of aiplatform's Mac footprint and 14 to 18% of the self build's; on
  Linux at 4 KiB, about 10% of aiplatform's max RSS (3.41 GB, decision 0629's host 1 row) and
  0.57 to 0.68 GB of the self build.
- the alloc bench and the HTTP server: under 5 MB. Their footprint is mostly the runtime's
  baseline and their live set, so this buys them nothing measurable.

## Decision (proposed)

1. **A pool of free pages per heap record.** A trim pass finds the small-class pages whose blocks
   are all free. It takes those blocks off their free lists and magazines, and gives the page to
   the record's free-page pool: page aligned, the whole page usable. Then it advises the page
   (MADV_FREE_REUSABLE on darwin; MADV_DONTNEED on Linux when the system page covers it, as
   decision 0533's advise does).
2. **Every class carves from the pool.** When a record's small bump (BUMP_CUR, BUMP_END) runs out,
   cabi.xo heap_grow takes a pool page (and the next ones, if they are adjacent) as its new bump
   run before decision 0383's cut and before it maps a chunk. So the page serves whichever class
   asks next: cross-class reuse at page granularity, which 0383 (a) can do only downward from
   donors under 32 KiB. A page touched again refaults as zero, which the allocator allows, because
   a block's contents are undefined.
3. **When the pass runs.** Only at the existing trim points: debug.mem_trim (xo calls it after
   front_end_freed, emit and link), each thread's own pass in pool.trim_all (each thread walks only
   its own record, so no list is read while its owner runs), and decision 0533's age pass (at most
   once per 30 s per idle worker). Never on the allocation path.
   - A record walks its lists only when its free small bytes pass a floor, for example 64 MiB, kept
     as a counter that moves only with whole magazines, not with every free. So a steady loop or a
     small server never pays the walk.
   - The walk marks pages in a per-chunk bitmap, the chunk's own table page (decision 0425's
     layout), not in a hash table.

## Cost (measured on the prototype, and what bounds the real one)

- The prototype's walk (single thread, a 32 MB hash table, both page sizes at once): 985 ms on
  aiplatform past emit (1.1 GB of free small blocks; the block count was not measured), about 5% of that build's
  21 s wall at one trim point. On `mixed` (3 MB free) it took 5 ms, almost all of it the table's
  fixed cost.
- The real pass would cost less: one page size, a per-chunk bitmap instead of a hash table, and
  each thread walking only its own lists on its own core at the trim (pool.trim_all already runs
  that way). Expected: a few hundred ms of CPU spread over the workers at xo's three trim points,
  to be measured. A program below the floor pays only the counter check.
- A steady-state allocation loop that never shrinks stays below the floor: it never walks. If it
  idles, 0533's age pass checks the counter at most once per 30 s.

## Gate (when the owner approves building it)

- The fixed point (s2 = s3); TestRuntime*; an allocation stress with cross-thread frees and the
  pool on and off (XO_PAGE_POOL=0 in the same binary).
- Peak footprint (Mac) and max RSS (linux/arm64 host 1, `xo bench --ab`): aiplatform with the
  default caches cold (each round in a fresh cache, gotchas/cache-rm-under-background-writer.md),
  and the self build.
- bench/perf/alloc (all workloads) and the HTTP server's rps and p99 under `--ab`, to show no
  cost where there is no gain.

## Consequences

- It lowers xo's peak where the free memory sits in whole pages: estimated 0.13 to 0.21 GB on
  aiplatform (Mac) and 0.36 to 0.41 GB on Linux, and 0.41 to 0.68 GB on the self build. It cannot
  reach the free bytes in pages that also hold a live block (about 80% of the free small memory).
- The allocator gets a third source for a bump run (the pool, then the cut, then a new chunk).
- Small servers and allocation loops see no change: nothing to reclaim, and no walk below the floor.

## Alternatives

- Advise the free small pages in place, without taking their blocks off the lists: it breaks the
  lists (each block's link and prefix are in the page).
- A shared page heap with size-segregated spans and coalescing (decision 0457, Rejected
  2026-11-02): it would reach the pages that mix lifetimes only if spans group blocks by lifetime.
  That is larger than this, and the owner rejected it once.
- Less churn in the compiler (arenas per function or per pass): it lowers the free memory itself.
  It is compiler work per pass, not runtime work, and complements this.
- Lower the floor to zero: every program would walk its lists at every trim, which is the cost the
  floor exists to avoid.
