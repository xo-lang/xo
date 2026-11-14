# 0633: A heap region for the compiler front end

Status: Accepted (owner, 2026-11-14, on the lead's recommendation; the open question answered: a
native-internal entry that only compiler/xo imports, no std/debug surface). Compiler and runtime only: no
language or std surface. Built 2026-11-14 (steps 1 and 2 below): max RSS on linux/arm64 0.914
(aiplatform test build) and 0.961 (xo self build), wall within noise.

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
3. The release works by 4 KiB page, not by chunk (step 0: the survivors left outside literals touch
   every 4 MiB chunk). With the region's own lists walked at the release (decision 0630's walk, which
   finds the pages free byte for byte; in the region nearly all of them are), its free pages form
   long runs: those are advised and pooled as runs (0630 v2's runs) that the normal heap's next bump
   runs take first.
4. The release (an explicit call, as decision 0630's scoping: xo calls it once the release task of
   0352 has ended, before insert_rc): the walk of point 3, then the region retires: its remaining
   free blocks (in pinned pages) stay on the region's lists for later front end use or none, and a
   later free of a survivor goes to them.
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

## Step 0 (measured 2026-11-14)

The same attribution on xo's own build (Mac, cold, caches off): front end allocated 1,150 MB live at
fe.build, 18.8 MB after the release (98.4% dies); other data 920 MB, then 783 MB. The survivors by
count (heap samples, inuse_objects, estimates):

| | aiplatform test build | xo self build |
|---|---|---|
| surviving objects | 199,000 | 169,000 |
| string literals (lexer.string_lit, cook) | 186,000 | 161,000 |
| others | 13,000 (display_name, scan.to_hex) | 8,000 (hostpath.clean_w) |

Pins: about 280,000 4 KiB pages of front end memory. With the literals in the region nearly every
page holds one, so they are allocated outside it (point 5). Without them, 8,000 to 13,000 survivors
pin at most 3 to 5% of the pages, but touch every 4 MiB chunk: hence the release by page (point 3),
and those three call sites allocated outside the region too where cheap.

## Steps

0. (Done, above.) Measure before code: the same profile attribution on xo's self build and on the HTTP CRUD
   server's build; and where the survivors would sit: with the front end's allocations in their own
   chunks, how many 16 KiB pages and 4 MiB chunks the 186,000 literals would pin (a probe that tags
   blocks by allocation mode).
1. Runtime: the region range, region records, the free path's routing, per chunk counts, the release.
   Behind an env switch (XO_HEAP_REGION=0 off) for the first --ab.
2. xo: region mode around the front end; literals out of it.
3. Gate (a default that changes what xo itself runs, CLAUDE.md): the defaults rule's full suites,
   the self build with the new xo, TestRuntime*, alloc and http --ab (no change expected: they never
   enter the mode), the amd64 sample, aiplatform and self max RSS --ab on linux/arm64 host 1.

## Steps 1 and 2 as built (2026-11-14)

- Records: region mode swaps the thread's heap record word to a region record (its own lists,
  magazines and bump, flagged H_RFLAG) whose central table is the region's (main central word 150).
  A pool thread entering the mode makes its normal record first.
- Range: a 16 GiB reservation (PROT_NONE), aligned to its size, from which region records carve
  their chunks. Each record carries the range's tag (address >> 34), or alloc.NO_TAG (2^40) when
  routing is off. The free fast path folds the test into the sign check it already makes:
  `(hdr | ((p >> 34) ^ tag) - 1) < 0`. free_mt and resize push a region block onto the freeing
  record's inbox (a CAS list) instead.
- Hooks in xo: check_program, the parse_prefetch tasks, Builder.check_lazy, and the builder's
  unparse of a body (region mode: trees parsed in the region must be freed into it, or the
  footprint got worse, 2.61 against 2.25 GB); Lexer.string_lit pauses the mode (internal ops 5
  and 6) so literals stay out.
- Release: at the strip point in lower, region mode drops the checker's last references, then a
  task beside the passes releases (ops 30 to 34). It takes hlock and stops the routing (every
  record's tag to NO_TAG), drains the inboxes, then walks: a per 4 KiB unit array over the carved
  range counts each unit's free bytes from every region list and central magazine (items taken in
  turns by XO_HEAP_REGION_WORKERS tasks, default 4, with atomic adds), unlinks the blocks of whole
  free units on the same workers, pools those units in address order as runs (advised a run at a
  time on Linux, not advised on darwin), and gives every other free block to the normal central
  lists. XO_HEAP_REGION=0 turns the region off (0352's release task as before);
  XO_HEAP_REGION_STATS=1 prints the release's counts.

Measured (Mac, aiplatform test build, cold):

| release | peak footprint, region on | region off | on/off | wall |
|---|---|---|---|---|
| synchronous, one walker | 2.01 to 2.04 GB | 2.19 to 2.21 GB | 0.92 | +300 ms |
| on a task, one walker | 2.118, 2.122 GB | 2.233, 2.243 GB | 0.946 | none |
| on a task, 4 walkers | 2.051, 2.090 GB | 2.261, 2.266 GB | 0.915 | none |

The region carves about 640 MB on aiplatform and pools 639 MB of it (survivors pin almost
nothing). The walk is about 385,000 free blocks on the region record's lists and 400 MB in the
region's central magazines; 4 walkers take it from 314 ms to 118 ms (a late release lets the
passes map new chunks first, which is why the one-walker task release lost against the
synchronous one). xo's self build: 0.98 (its peak is emit's).

Linux/arm64, host 1, max RSS, cold, region on (4 walkers) against XO_HEAP_REGION=0, alternating,
medians of 5 (d47611f11; the ranges do not overlap):

| | region on | region off | on/off |
|---|---|---|---|
| aiplatform test build | 2.078 GB (2.058 to 2.124) | 2.274 GB (2.252 to 2.302) | 0.914 |
| xo self build | 3.855 GB (3.828 to 3.900) | 4.012 GB (3.956 to 4.047) | 0.961 |

Release 117 to 178 ms on aiplatform, 164 to 233 ms on the self build (the wall times of the
alternated runs above overlap and decide nothing).

`xo bench --ab` on host 1, main (719edcf26) against the region branch (517dfdd6a):

| program | wall B/A | cpu B/A | max RSS B/A |
|---|---|---|---|
| xo's self build (stage2 self=a: each side's xo built by itself, both build main's compiler), 5 rounds | 1.006, within noise (0.993 to 1.007) | 0.996 | 0.964, better (0.947 to 0.975) |
| bench/perf/alloc (never in region mode: the free path's folded test), 15 rounds | 0.981, within noise (0.95 to 1.041) | 0.982 | 1.0 |

The self build ran with the host at load about 6 (the bench warned); a quiet rerun is a check, not
a gate.

## Consequences

- One test on every free, folded into the sign test the fast path already makes (alloc and an
  HTTP keep-alive server within noise under --ab).
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
  keeps std's surface unchanged. Answered (owner, 2026-11-14): the native-internal entry, no std/debug
  surface.
- Native-internal for now, revisited if the region proves itself (lead, 2026-11-14): the runtime's
  entry keeps a stable shape (region on or off, and the release) so a later std/debug wrapper is
  thin; after the footprint result, a follow-up ADR weighs a public hook against decision 0524's
  request arenas. Stage 3 of decision 0637 lowers bodies on worker tasks, whose builder copies are
  front end allocations too: the region covers the workers' threads while they run.
