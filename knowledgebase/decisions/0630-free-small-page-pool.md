# 0630: Free small pages: whole free pages of the small classes leave their free lists for a pool every class carves from

Status: Accepted (owner, 2026-11-14), scoped to an explicit call (xo after emit), no automatic trigger in user programs; built 2026-11-14, measured negative on xo (11 to 18 MB pooled, footprint unchanged, wall +12 to 19%), and not shipped (see "Built and measured"). Proposed by Xo Lang 1, 2026-11-13

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

## Built and measured (2026-11-14): negative, not merged

Branch lang1-0630 (unpushed). An explicit trim walks each thread's own lists (on its own thread, through
pool.trim_all's epoch) and then the central magazines (from the mem_trim caller, every small class's lock
taken in class order). It finds 16 KiB units that are free blocks byte for byte (a union-find over units that
blocks straddle, since a block is never split), unlinks their blocks, advises the units, and pools them. The
pool is a side array, never written into the advised units. heap_grow takes a pooled unit as a bump run before
a cut or a new chunk (H_BLIM 1, so extend never grows it). XO_PAGE_POOL=0 turns it off.

- Correct: two stress programs, single threaded and 16 tasks, over four rounds of a 1/97 keep. 0 bad
  records; 13,578 and 18,986 units pooled; their memory reused by later allocations of other classes.
- The cost is the walk: pointer chasing over every free small block, about 0.3 s per trim for 10 M blocks on
  one thread. A first fixed-point pass over straddlers took 55 s (a propagation of one unit per full walk) and
  was replaced by the union-find. With the central class locks held for the walk, workers block on them.
- On xo (Mac, aiplatform test build, default caches cold, the same binary with XO_PAGE_POOL=0 and =1, two
  rounds each): 729 and 1,122 units pooled (11 to 18 MB). Peak footprint 2.70/2.72 GB against 2.68/2.70 GB
  (no change). Wall 20.2/21.1 s to 22.7/25.1 s (+12 to 19%): link 0.24 to 1.6 to 1.8 s, emit +1.3 to 2.3 s.
- Why so few units: a thread's own lists hold at most about two magazines a class (under the 16 MiB floor,
  so each thread's walk skips). Most free blocks are in the central magazines, but a unit qualifies only
  when one source's blocks cover all of it. The census (which counted every list at once, from one thread,
  unsafely) found 210 MB of whole units at 16 KiB. Walking every list together needs every thread
  stopped for the walk.

So the decision stands as scoped, but this build is not merged. Next, ranked by expected gain:
(1) cut the live heap at emit directly: raw.str_join 255 MB, fcache.subst_tokens 227 MB and fcache.put_field
251 MB at "emit start 100" (lang1 notes emit-stream-design.md), the allocations behind the peak; (2) a
stop-the-world variant of this pass (every worker parked at xo's trim point, one walk over all lists), whose
ceiling is the census's 210 MB (about 7% of the footprint) for about 1 s per trim; (3) segregating the bump by
lifetime (part-local allocations in their own chunks, freed whole after the part), which makes whole free
units by construction instead of finding them.

### Version 2 (2026-11-14): 4 KiB units, runs, one walk after the release; negative

Branch lang1-0630 at 602766e2b (unpushed). Units of 4 KiB with no taint rule (a block that touches a taken unit
leaves its list), the pool sorted and kept as runs (addr | length, up to 4,095 units), heap_grow taking a run of
the units it needs before the bump extends, and one explicit trim (debug.mem_trim_pool) that xo calls at
"specialize_args", after the front end's release (decision 0352) has run. On the Mac it pooled and reused 223 MB;
436 of the 659 MB free at that point lay in units that still held a live block.

gcp aiplatform/apiv1 test build, linux/arm64 host 1, `--ab` against its base, 9 rounds, 0 loaded: max RSS 0.995
(0.981 to 1.014, within noise), wall 1.097 (within noise), CPU 1.089 (1.01 to 1.141, worse). A free block taken
into the pool would mostly have been reused by its own class anyway. Not merged; the lifetime split this points
to is decision 0633.

## Consequences

- It lowers xo's peak where the free memory sits in whole pages: estimated 0.13 to 0.21 GB on
  aiplatform (Mac) and 0.36 to 0.41 GB on Linux, and 0.41 to 0.68 GB on the self build. It cannot
  reach the free bytes in pages that also hold a live block (about 80% of the free small memory).
- The allocator gets a third source for a bump run (the pool, then the cut, then a new chunk).
- Small servers and allocation loops see no change: nothing to reclaim, and no walk below the floor.

## Alternatives

- Explicit scan only (the lead's recommendation to the owner, 2026-11-13). The pool and the
  trim code ship in the runtime, but only an explicit call runs the scan: debug.mem_trim, which xo
  calls at its phase boundaries (after front_end_freed, emit and link). Nothing triggers it in a
  user program, so 0533's age pass and pool.trim_all's idle trims do not scan. The reasons: user
  programs gain under 5 MB (the alloc bench and the HTTP server above), while the scan has a real
  cost. The prototype's walk takes 985 ms on aiplatform past emit, about 4.7% of that build's
  21 s wall (Mac, caches off). The real pass is expected to cost less (per thread, a bitmap, one
  page size) and is measured before it lands. A program that wants the memory back calls
  debug.mem_trim itself, as xo does. This variant needs no free-bytes floor, because a program
  that never calls the trim never pays anything.
- Advise the free small pages in place, without taking their blocks off the lists: it breaks the
  lists (each block's link and prefix are in the page).
- A shared page heap with size-segregated spans and coalescing (decision 0457, Rejected
  2026-11-02): it would reach the pages that mix lifetimes only if spans group blocks by lifetime.
  That is larger than this, and the owner rejected it once.
- Less churn in the compiler (arenas per function or per pass): it lowers the free memory itself.
  It is compiler work per pass, not runtime work, and complements this.
- Lower the floor to zero: every program would walk its lists at every trim, which is the cost the
  floor exists to avoid.
