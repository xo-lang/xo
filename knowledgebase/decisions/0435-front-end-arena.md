# 0435: Arena mode for the front end's trees: no-op frees, dead runs swept at the strip

Status: Accepted (owner, 2026-10-30; design by xo-lang-6, 2026-10-30, after items 2 and 3 of
the memory plan, decisions 0418 and 0425); implemented and measured 2026-10-30 (xo-lang-6, branch xl91, 1e3e9f4bb, kept as refs/recover/xl91-arena, not merged): a negative result, the expected footprint win is not there, see Consequences (measured) and Next paths.

## Context

The self build's footprint holds the parser's and checker's trees past the point they are needed.
The trees are about 1.5 GB live at the end of the front end, mostly 64 byte blocks (mem6 probe,
2026-10-29; mem-plan.md). They are freed at the strip (compiler/xo main.xo, phase
"front_end_freed", on a spawned task after ssa), but their size class is never asked for again:
the later passes (SSA lowering, the backends, the linker) take 2 KiB and larger blocks. A class's
free blocks lie scattered through the mixed-class 4 MiB chunks, and decision 0383's allocator never
coalesces a class (the cut only carves downward), so none of their pages is ever given back:
1.39 GB of 64 byte blocks are free at exit. The scavenge of decision 0383 returns only the whole
pages inside big free blocks, and the page blocks of decision 0425 only reach the 16 KiB and larger
classes. No native arena exists in std/native/llrt.

Measured 2026-10-30 04:18 (self build with item 3's xo, cache off, heap profile at the end of
check, 16 MiB sampling rate): the front end allocates 3.79 GB in 11.3M objects for 1.60 GB in
5.5M objects live (2.4 times the live set; heap 2086 MB at check, peak 2273 MB). The parser's
trees are about 1.0 GB of the live set (parse_fn_decl 912 MB cumulative: parse_block 800,
parse_binary 672, parse_primary 592, parse_if 416, parse_match 208; the lexer's string literals
96) and the checker's about 0.5 GB (call_method 320 cumulative, infer0 419, let_stmt 224,
expr_stmt 192, check_args 112, declare 64). The churn is the checker's transient lists (check0
1.00 GB cumulative for 448 MB flat, pre_expr 337 MB) and the parser's lists grown then copied
(parse_block 1.06 GB cumulative for 240 MB flat, parse_if 624 for 256, lexer.emit 193 MB).

## Decision

An arena mode on the heap record, toggled by the driver around the front end, with the dead
runs given back by a sweep at the strip. No pointer ever moves.

1. Chunks are 4 MiB aligned with a one page header (decision 0425's map_aligned and table page;
   a plain chunk's header holds its kind and bounds, 0.4% of the chunk), so every block finds its
   chunk by `p & -4194304`.
2. A heap record word ARENA, set on every record from a global the driver toggles through
   rt.heap_grow(h, ARENA_ON / ARENA_OFF) flags (the seed workaround of decision 0383: the seed
   knows rt.heap_grow, not a new rt name). With it on, alloc skips the free lists and carves every
   block from the record's arena bump (ACUR, AEND; a chunk of kind arena per record, listed in
   heapos under W_ARENAS), writing prefix 127 (ARENA) with the class in bits 8 to 15 instead of the
   class alone. Classes and sizes stay the same, so every reader of a block's size (resize, the free
   masks, the heap profile) reads the class from bits 8 to 15 when the low byte is 127.
3. free of an arena block (low byte 127) writes 126 (DEAD) into the low byte and returns: no list.
   free_push does the same and returns true. resize of an arena block is in place when its class
   serves the new size (as today); otherwise the caller moves it (alloc plus copy, the old block
   marked dead).
4. The strip (after fe.build and ssa: the checker dropped, the link view has copied what it
   keeps) calls rt.heap_grow(h, ARENA_SWEEP): heapos walks every arena chunk block by block (the
   prefix gives the class, the class the stride), finds the runs of dead blocks and madvises the
   whole pages inside each run (Darwin MADV_FREE_REUSABLE, Linux MADV_DONTNEED, as the scavenge);
   live blocks (prefix 127: the module's names, types, strings and the SSA built while the mode was
   on) stay where they are. A chunk with no live block is unmapped, or kept as the next arena
   chunk. Later frees of arena blocks mark them dead; a second sweep at hollow returns those.
5. Sampling (heapprof.xo negates the class): an arena block's negative prefix is -(127 | c<<8),
   which heap_big_release's "sampled class" test (c > -128) does not match; the arena case is added
   there (forget the sample, mark dead). cut_for, recount, nfree and the scavenge never see arena
   blocks: they are on no list.
6. The driver (compiler/xo main.xo): ARENA_ON before parse, ARENA_OFF after fe.build (or after
   ssa), ARENA_SWEEP on the strip's release task and again at hollow. The front end's worker
   threads (the parallel check of decision 0271) take the word from the global at their heap
   record's creation and at the toggle, which walks W_HEAPS under hlock as profile_on does.
7. Shape, from the volume measurement above: the plain bump arena would hold the front end's
   3.8 GB of volume at the end of check against 2.1 GB today, so the mode covers the parser alone
   first (ARENA_ON around parse_file on each worker; its trees are 1.0 of the 1.6 GB live and its
   churn about 1.5 times its live set), with the checker on the normal heap. If the sweep's own
   count (the XO_HEAPSTATS "arena" line: chunks, bytes live, bytes returned) shows the checker's
   0.5 GB worth it, the second step adds per-class free lists on the heap record inside arena mode
   (a freed arena block goes back to the arena's own list, reused by arena allocations only; the
   sweep still returns the dead runs that remain) and covers the whole front end.
8. Diagnostics: XO_HEAPSTATS prints "arena: N chunks, M bytes live, K bytes returned" after each
   sweep.

## Consequences (as designed)

- The self build's footprint drops by the parser's trees minus the survivors' pages (names, types
  and strings kept by the link view: a few hundred MB), given back at the strip before emit's
  peak: the expected win is 0.8 to 1.2 GB (to be measured: the self build's peak RSS before and
  after, and an --ab of the self build on the linux/arm64 host per the owner's rule of
  2026-10-30). Small builds (fib) are unchanged within noise: one arena chunk.
- Cost: one word load and branch per alloc while a heap record exists (the ARENA test on alloc's
  fast path; --ab on alloc, json and the self build), the sweep (about 16M blocks for the parser's
  1.0 GB: tens of ms on the strip's task) and the chunk header page per chunk.
- A block freed in arena mode is not reused until the sweep (step 7 keeps the parser's churn, about
  1.5 times its live set, inside the arena; the lists of the second step bound it by the live set).
- A new block kind (prefix 127 and 126) that every reader of the prefix must know: resize, the
  free masks, the heap profile and heap_big_release. The llrt tests cover alloc, free, resize and
  sweep of arena blocks and a profile sample inside the arena.
- Gate (an allocator change that xo itself runs): the llrt tests, the fixed point (s2 == s3), a
  self build with the new xo (scripts/bootstrap-xo.sh s2), identity against the parent, --ab on
  alloc, json and the self build on the host, the footprint A/B of the self build, and the
  XO_HEAPSTATS arena line.
- Builds on decision 0425's aligned chunks; lands after it.

## Consequences (measured, 2026-10-30)

- No footprint win on the self build. linux/amd64 (28 CPUs, cache off, the arena on against
  off, both twice): max RSS 7685 MB against 7907 and 7742 MB; `xo bench --ab` on the amd64
  host (BASE's compiler/xo built by both xos, 5 rounds, busy machine): wall 0.984 (0.876 to
  1.143, within noise), CPU 1.05 (within noise), max RSS 0.97 (7611 to 7380 MB, better);
  the same on the linux/arm64 host (12 CPUs, quiet, 0 of 5 rounds loaded): wall 1.001 (0.984
  to 1.01), CPU 0.983 (0.98 to 1.01), max RSS 0.993 (7913 to 7854 MB), all within noise.
  darwin (the Mac, cache off): max RSS 7845 MB with the arena against 7461 MB without, 384 MB
  WORSE (MADV_FREE_REUSABLE pages stay resident until reused or under pressure), wall 64.9
  against 66.6 s. The fixed point (s2 == s3) held and the arena changed no output.
- Why the premise failed: the parser's trees are 2.1 GB at the end of check and the sweep
  found only 3 MB of them alive after the strip (496 chunks, 3112 MB given back), so the
  mechanism works; but with the lists, the trees' freed blocks are reused by the passes and
  emit, whose heap grows past them (emit's heap 8.7 GB mapped), so their scattered pages never
  cost at the peak. The peak is emit's live set either way; giving the trees' pages to the OS
  only makes the later phases map fresh chunks.
- A cost the premise hid: the budget's mapped figure (decision 0349, debug.mem_mapped) is 1.7
  GB worse with the arena (peak 12228 MB against 10496): the 496 arena chunks stay mapped
  with 3 MB of survivors scattered through them, as the scavenger's madvised pages do.
- Two traps found, kept as gotchas for the next allocator change: a chunk filled to its last
  byte makes `cur & -CHUNK` the next region's base (chunk-base-from-exact-fill.md), and a
  compiler call of internal.rt_probe pulls sys_c.qsort into the static Linux link
  (rt-probe-pulls-qsort-into-static-link.md: the compiler got internal.arena_mode). A task's
  captures die when the task ends, not at their last use (core 7.1): a sweep inside the
  release task saw every tree alive.
- Where the peak is (XO_PROF=heap at one sample per 16 MiB, XO_PROF_HEAP_AT, linux/amd64,
  the self build with lists): at the end of inline.verify (emit's start) 3.86 GB in use:
  ssa.renumber (passes.xo:1256) 1.85 GB, ssa.Func.add_param (ir.xo:867) 835 MB,
  arm64.inline_call (inline.xo:1082) 406 MB flat and 1.38 GB cumulative, then the passes'
  lists (merge_straight, fold_tuple_fields, cse, 100 MB each); at the end of emit 728 MB in
  use (712 MB in arm64.drop_weak_in, calls.xo:3793) while the heap holds 8.7 GB mapped: emit
  adds about 3.8 GB of transient allocations over the SSA (mapped peak 10.7 GB against 6.9 GB
  before it) and frees them, and the SSA is dropped at hollow. So the self build's peak RSS
  is the SSA's 3.9 GB plus emit's transient.

## Next paths (ranked by the peak, 2026-10-30)

1. ssa.renumber's 1.85 GB (passes.xo:1256): what it keeps (the renumbered value lists; does
   the old Func survive beside the new one until the pass ends?). Experiment: XO_PROF=heap
   with XO_PROF_HEAP_AT=passes on a build of bench/perf/json, then the self build; drop the
   old lists in place.
2. emit's transient 3.8 GB: stream the emit (encode a function and free its buffers before
   the next, keeping only the object bytes), bounding the transient by the emit jobs' count
   times the largest function. Experiment: XO_EMIT_JOBS=1 against 8 under `--phase-times`
   (the mapped peak at emit) to see how much of the transient is parallel slack.
3. ssa.Func.add_param's 835 MB (ir.xo:867): the block parameter lists' capacity slack
   (every List growth keeps up to 2x); pack after the passes.
4. The arena's second shape (free lists inside arena mode) would halve its chunk count but
   not change the verdict: the trees are not at the peak.

## Alternatives

- Coalescing free 64 byte blocks into pages (a class that gives whole pages back): rejected, as
  in decision 0383; the free blocks of a class lie scattered through mixed-class chunks, so no
  page becomes free without moving live blocks.
- Moving the trees (a copying arena): rejected; every pointer into a tree would need a fix-up
  and the checker keeps pointers in maps and closures.
- A plain bump arena over the whole front end with no reuse: rejected by the measurement; the
  front end's volume is 2.4 times its live set, so the arena would hold 3.8 GB at the end of check
  against 2.1 GB today.
- Freeing the trees sooner (dropping each module's tree once its SSA is built): the link view
  still needs the names and types, and the win is bounded by the same scattered pages; may be
  combined later.
- A separate process for the front end: rejected; the SSA and facts would cross a process
  boundary at every build.
