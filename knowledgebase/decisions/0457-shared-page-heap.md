# 0457: A shared, address ordered page heap for page blocks, large blocks and the threads' runs

Status: Proposed (Xo Lang 2, 2026-11-02; the lead's item after decision 0456; needs the
owner's acceptance, allocator change). Stage 1 is a prototype to measure, not a default.

## Context

bench/results/thp-density-2026-11-02.md measured where the gap between Go's and Xo's memory
comes from under THP `always`. Go fills the 2 MiB regions it touches (71-89%); Xo fills 13-23%.
With 2,000 held TLS connections Xo takes 95-121 MB and Go 80-88. XO_THP=off brings Xo to
40-65 MB, but costs the self build 7.5% CPU (decision 0454). Decision 0456 (XO_THP=dense)
keeps the self build's speed, but leaves an idle server at off's memory and a short build
paying 4%.

The cause is the shape of Xo's heap, not the kernel. Every heap record (one per thread that
allocates) owns a 4 MiB bump chunk for blocks up to 8 KiB, and a 4 MiB page chunk for page
blocks of 16 to 256 KiB (decision 0425). It also maps each large block (over 256 KiB) on its
own, keeping up to 16 freed mappings of 64 MB in all in a cache. A thread's chunks are as
full as that thread made them: 2,000 connections on 18 threads leave about 260 KiB used in
each 4 MiB chunk. A TLS handshake burst maps 148 MB of chunks against 64 for plain
connections (Xo Lang 3, 2026-11-01). Nothing that is freed goes back to a shared pool that
another thread or size could use. Decision 0383's cut reuses central magazines across small
classes, but not pages.

Go (runtime/mheap.go, mpagealloc.go, mgcscavenge.go):
- One page heap per process, its pages found through a radix tree of bitmaps, first fit at
  the lowest address, so live data packs toward the bottom and the regions it touches fill.
- Each P keeps a 64-page cache taken from the heap under its lock.
- Spans of one size class come from the heap and go back when they are wholly free.
- A background scavenger returns free memory whole huge pages at a time, never splitting
  one, toward a retained target of the heap goal plus about 10%.

Decision 0450 (owner, 2026-11-01) declined spans for the small classes: their free blocks sit
among live ones, and separating classes freed under 2% of the self build's peak. This
decision does not revisit that. Small blocks keep their prefix, classes, lists, magazines and
cuts. What changes is where every thread's memory comes from.

## Decision

A page heap shared by every thread, with page sized runs, takes over:

1. **Arenas.** Address space is reserved in 64 MiB arenas, each aligned to 4 MiB. Each 4 MiB
   chunk of an arena keeps decision 0425's layout: its first page is the table of its 16 KiB
   pages, and its first word is PAGE_MAGIC. alloc.xo's is_page, free paths and table words are
   therefore unchanged. A per chunk bitmap marks each page free or in use, and a second bitmap
   marks it scavenged. A per arena summary keeps each chunk's free page count and longest free
   run.
2. **Allocation.** A run of n pages is taken first fit at the lowest address: the summaries
   are scanned first, then the bitmaps of the candidate chunk. One lock covers the heap
   (W_PLOCK, a pool lock like the central lists'). Each heap record keeps a page cache of up
   to 64 pages (1 MiB) inside one chunk, which serves single pages without the lock, as Go's
   pageCache does.
3. **What draws from it.**
   - Large blocks: a run of whole pages. The prefix stays, so free still finds them; release
     returns the run, and adjacent free runs coalesce through the bitmap. This replaces the
     per block mapping and its cache.
   - Page blocks: a record's page bump (PCUR, PEND) takes runs of 64 KiB, doubling to 1 MiB,
     instead of a 4 MiB page chunk.
   - Small blocks: a record's bump (BUMP_CUR, BUMP_END) takes runs the same way: 64 KiB first,
     doubling to 1 MiB while the record keeps filling them, instead of a 4 MiB chunk.
4. **Returning pages.** When a page class's central magazines pass a bound, whole page blocks
   go back to the page heap and coalesce, instead of staying with their class. Small blocks
   never do (decision 0450).
5. **Scavenger.** A scavenge returns free runs that cover whole 2 MiB aligned regions:
   MADV_DONTNEED on Linux, MADV_FREE_REUSABLE on darwin. It runs at the existing trim points
   and from the sysmon tick when retained free pages pass the target (live heap plus 10%, at
   least 16 MiB). It never splits a region the heap touches. A partial region is returned only
   when the memory budget (decision 0349) asks for room.
6. **Huge pages.** Regions fill bottom up, so the kernel's default policy is kept. XO_THP=off
   and dense keep their meanings for the runs they map.

### Stages, each measured before the next

1. Large blocks and page blocks from the page heap (items 1 to 3 without small blocks; no
   scavenger beyond today's). This is the prototype.
2. Page classes return whole blocks to the heap (item 4).
3. Small block runs (item 3's last point): the stage that should change an idle server.
4. The scavenger (item 5).

Every stage is measured on: 2,000 held TLS connections (RssAnon and AnonHugePages under
`always`, WSL with THP set for the run and the second linux/arm64 host); `xo bench --ab` of
self, coll edit and alloc on the first host; and the Mac's coll warm footprint. A stage lands
only with self wall within 1% and memory down on at least one of them.

## Prototype results (2026-11-02)

Xo Lang 2 built stages 1 to 4 on unpushed branches (l2-pheap to l2-pheap16; the table of
branches, the logs and every number are in bench/results/pheap-2026-11-02.md). Correctness
held: the goldens with XO_LEAKCHECK, TestRun and the XO_RC_CHECK goldens pass on linux/arm64 at
stage 4, and Xo Lang 1's stress program (golden pheap_stress on l2-pheap8) is clean on
linux/arm64 and darwin.

- Idle server, 2,000 held TLS connections, THP always: stage 3 took Xo from 108-138 MB to 64-69
  (Go 85-95). Decision 0456's v4, landed the same day, takes it to 60-64 on its own, and the
  page heap on top of v4 measures 65-71: no further gain.
- Self build, linux/arm64 (THP always), against its base: stage 1 wall 1.007, max RSS 1.027;
  stage 3 wall 1.03, max RSS 1.058; all four stages wall 1.037 to 1.065 and max RSS 1.112 to
  1.171 by the scavenger's goal; on top of 0456 v4 (l2-pheap16) wall 1.019, CPU 1.03, max RSS
  1.041 against v4 alone.
- Darwin self build footprint: 3.99-4.15 GB against 3.62-3.69 with every variant tried.
- Cost: the scavenger released 13-16 GB over one self build and most of it faulted in again
  (a goal tight enough for RSS costs wall), and reused runs keep pages that a fresh mapping of
  a list's unwritten capacity never touches.

Recommendation (Xo Lang 2): do not land the page heap in this form. With 0456 v4 the density it
was for is reached by advice on the existing chunks, and every stage measured costs the self
build. The pieces worth keeping if the question reopens: the per chunk longest free run
(PT_MAX), the lock free switch read, and the stress golden. Whether to reject or park this
decision is the owner's call; the status stays Proposed until then.

## Consequences

- Freed pages serve any later large or page block, and any thread's runs. A thread that
  allocates little holds a 64 KiB run, not 8 MiB of chunks.
- The page heap's lock is a new shared point. The per record page cache and the run doubling
  keep it off the small block fast path, which takes no new instruction: runs only change what
  BUMP_END and PEND point at.
- Sanitizer and --race builds keep today's paths (decision 0425 point 5: their large blocks
  come from the C library).
- The large mapping cache (CACHE_N, cache_drop, debug.mem_trim's mappings, decision 0448
  case (c)) goes away in stage 1. mem_trim becomes a scavenge.
- Windows reserves arenas and commits runs (VirtualAlloc MEM_COMMIT); its granularity is
  64 KiB, the first run size.

## Alternatives

- XO_THP=off or dense only (decisions 0454, 0456): they fix the kernel's side for a sparse
  heap, not the sparseness. An idle server stays at off's memory, and a short build pays.
- Spans for small classes: declined (decision 0450).
- Smaller fixed chunks, with no shared heap (decision 0455's 256 KiB first chunks, darwin):
  they lower the count of mapped memory, but freed memory still stays with its thread and
  class. On Linux they cost conc-spawn 2 to 3% with no RSS gain.
- A per arena allocator from a library (mimalloc's segments and pages): close to stages 1 to 3
  in shape. Xo's runtime is in Xo with no C library dependency (zero-C), so it would be a port.
  Not chosen over Go's simpler first fit, whose behavior the density study measured.
