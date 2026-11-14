# 0633: A heap region for the compiler front end

Status: Proposed (Xo Lang 1, 2026-11-14). Compiler and runtime only: no language surface beyond the
std/debug hook named in Open questions.

## Context

xo's build footprint is fragmentation, not live data. On a cold test build of the gcp
aiplatform/apiv1 SDK (Mac, main b98479d92, default caches), 612 MB is live at emit's start while the
heap maps 2.2 GB and the peak footprint is 2.5 GB. The census probe (lang1 notes, 2026-11-14) shows:

- Every free small block sits on the central magazines: 470 MB after the front end's release, 484 MB
  after the inliner, 1,114 MB after emit.
- That memory is too fragmented to reuse across classes. Decision 0630's pool, rebuilt with 4 KiB
  units, runs, and a walk once the release had run, reused 223 MB, and the footprint stayed within
  noise: a free block taken for the pool would mostly have been reused by its own class anyway, and
  436 of the 659 MB free at that point lay in units that still hold a live block. The cut of larger
  classes before a new chunk (0383 (a) run first) gained 20 MB.
- Smaller diets help a few percent each: giving generic bodies' trees back after each instance
  (6fcf6191e) took the footprint from 2.53 to 2.48 GB.

The lifetimes are what decide it. Heap profiles at "past phase fe.build" and "past phase
specialize_args" (after the front end's release, decision 0352), with each live sample attributed
by whether a check, parser, or lexer frame is on its allocation stack:

| | front end allocated | allocated elsewhere |
|---|---|---|
| live at fe.build | 1,113 MB | 355 MB |
| live after the release | 26.6 MB | 344 MB |

97.6% of the front end's bytes die together. The survivors are 16 MB of file source buffers (few,
large blocks) and about 186,000 string literals (lexer.string_lit and cook: 9 MB, about 50 bytes each,
kept by the SSA), plus a few display names. Today these 1.1 GB are interleaved with the SSA's
allocations in the same chunks, which the later phases' growth (insert_rc and the inliner +600 MB,
emit +220 MB) cannot reuse across classes.

## Decision (proposed)

The front end allocates in a heap region of its own, and the region's chunks come back whole once
the front end is released.

1. The region: a reserved address range (untouched until used, like the bump chunks' reservation),
   carved into chunks as today's, with one heap record per thread for it (its free lists, magazines
   and bump). A thread in region mode uses its region record: the mode switches the thread's heap
   record word, so the allocation fast path does not change.
2. Frees route by address: a block inside the region's range goes to the freeing thread's region
   record (one compare on the free path, predictable), so a front end block never enters a normal
   free list and no later allocation reuses it from there.
3. Per chunk live counts for the region only (an increment on a region carve, a decrement on a region
   free, atomic since frees cross threads). A region chunk whose count is 0 after the release has no
   live block.
4. The release (an explicit call, as decision 0630's scoping: xo calls it once the release task of
   0352 has ended, before insert_rc): the region's lists are dropped, every chunk at count 0 is given
   to the normal chunk list as a fresh chunk (advised: MADV_FREE_REUSABLE on darwin, MADV_DONTNEED on
   Linux, until the normal heap carves it), and the region retires (a later free of a survivor only
   decrements its chunk's count; a chunk reaching 0 then is given back the same way).
5. xo enters region mode around the check (check_program, the parse) and around the SSA builder's
   lazy checks and parses (check_lazy, force_body); it leaves it around the allocations that
   survive: string literals the SSA keeps (lexer.string_lit and cook) and the file sources, which
   are large blocks outside the small chunks anyway.

Expected, aiplatform test build: about 1.09 GB of front end allocations, all but the pinned chunks
given back; the later phases' growth (about 0.8 GB of mapped heap after the release) carves from
them instead of new chunks, so the mapped heap at emit falls from 2.2 GB toward 1.5 to 1.7 GB and the
footprint by 20 to 30%, if the literals do not pin. On xo's own build the peak is emit's (mapped
3.98 GB against 2.08 at fe.build); step 0 measures how much of its front end memory the same split
would return.

## Steps

0. Measure before code: the same profile attribution on xo's self build and on the HTTP CRUD
   server's build; and where the survivors would sit: with the front end's allocations in their own
   chunks, how many 16 KiB pages and 4 MiB chunks the 186,000 literals would pin (a probe that tags
   blocks by allocation mode).
1. Runtime: the region range, region records, the free path's routing, per chunk counts, the release.
   Behind an env switch (XO_HEAP_REGION=0 off) for the first --ab.
2. xo: region mode around the front end; literals out of it.
3. Gate (a default that changes what xo itself runs, CLAUDE.md): the defaults rule's full suites,
   the self build with the new xo, TestRuntime*, alloc and http --ab (no change expected: they never
   enter the mode), the amd64 sample, aiplatform and self max RSS --ab on linux/arm64 host 1.

## Consequences

- One compare on every free; a region block's free also decrements a counter.
- A survivor left in region mode by mistake pins its chunk (4 MiB) until it is freed: a missed
  survivor costs memory, never correctness. The probe of step 0 and XO_HEAPSTATS (pinned region
  chunks) watch it.
- Thread migration: a task that moves threads mid-mode switches the new thread's record word too
  (the mode is a property of the task while it runs front end code; xo's front end tasks do not
  yield inside a parse or a check).
- Decision 0630's pool and decision 0383's cut stay as they are: they act on normal chunks.

## Alternatives

- Smaller front end data (records, effect sets, trees): 1 to 3% each, measured or estimated
  (lang1 notes); they lower the fe.build peak but the freed memory stays fragmented.
- Decision 0630's pool at 4 KiB (built, 2026-11-14): bounded by the survivors interleaved in the same
  units, within noise.
- A copying step at the release (move the 344 MB of non front end data out): needs every pointer
  found, which the runtime does not track.
- Language level arenas (consider-later; decision 0524 for request arenas): broader; this one is
  internal to the compiler and the runtime.

## Open questions

- The hook's surface: a std/debug function (debug.heap_region(on: Bool) and debug.region_release(),
  spec std.md, nothing on the Go and wasm backends) usable by any program with phase lifetimes, or a
  native-only internal entry that only compiler/xo imports. The first is simpler to test; the second
  keeps std's surface unchanged.
