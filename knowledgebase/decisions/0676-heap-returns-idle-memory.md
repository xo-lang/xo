# 0676: the native heap returns idle memory to the OS

Status: Accepted (lead for the owner, 2026-11-16 overnight; proposed by Xo Lang 1, 2026-11-16; task #91)

## Context

The sql stress program (std/evidence/sql/stress) grows from 56 to 88 MB
resident over 30 minutes on linux/arm64 with one live block at the end
and 68 of 92 MB on the size class free lists; Go's twin stays near 25 MB
(aws-43, 2026-11-16, 562cbe72e). It blocks std/sql's stress evidence
(Proven).

Decision 0533 gives back the pages of free page blocks (classes of 16 KiB
and up) from an age pass an idle worker runs every SCAV_EVERY: a block
free on two passes in a row is advised. It left the classes under 16 KiB
(the prefixed blocks: an 8 byte class prefix, the free list's link in the
block's first word) to "a separate decision": their blocks are carved
from shared 4 MiB chunks, every class from the same bump pointer, so a
page holds blocks of several classes and nothing counts a page's free
blocks. This is that decision.

What the free lists hold, measured (Xo Lang 1, 2026-11-16, the Mac,
`XO_HEAPSTATS=pages` added for this, a 5 minute stress run at
758168f19): 164,751 free blocks of the prefixed classes, 55.7 MB in 8,499
address-contiguous runs. 50.1 MB of it (90%) is whole 4 KiB pages
covered by free blocks only, 41.1 MB (74%) whole 16 KiB pages. The
classes spread evenly (32 B to 5 KiB, 0.1 to 1.8 MB each): a burst of
work carves a stretch of a chunk, and when the burst ends the stretch is
free together. So most of the retention is reachable at page level
without knowing which class owns a page. The 30 minute Linux run is
measured the same way in stage 1 before stage 2 starts.

## Decision

Extend the age pass (heapos.xo age, pool.xo age_check) to the prefixed
classes with a page sweep:

1. **Find whole free pages.** On the calling thread's heap record (its
   lists and spare magazines, owned by the thread, so no lock) and, when
   central, each class's central magazines under that class's lock: copy
   the free blocks' ranges ([address - 8, address + size)) into a scratch
   area (rt.heap_large, released after), sort them by address (an in
   place heap sort: llrt allocates no Xo values), merge adjacent ranges
   into runs, and keep the whole OS pages inside each run (AT_PAGESZ on
   Linux, 16 KiB on darwin). At most SWEEP_MAX blocks a pass (65,536), so
   one pass stays near a millisecond; the next pass takes the rest.
2. **Age them as 0533 does.** A pass records the whole free page ranges
   it found in a table beside the heap record; the next pass gives back
   only pages that are whole free again and were in the previous table.
   So a page goes back only after a whole period free, and churn between
   phases is not refaulted.
3. **Unlink and give back.** Every free block that touches a page given
   back leaves its list (each list rebuilt by one walk that keeps the
   blocks outside the released ranges). The pages are advised (0533's
   advise: MADV_DONTNEED on Linux, MADV_FREE_REUSABLE on darwin), and the
   released run, with the pieces of its blocks outside the whole pages,
   goes on the heap record's run list (address ordered, a start and end
   word per run, in the table of step 2: a released page holds nothing).
4. **Reuse runs before chunks.** When the bump chunk runs out, a thread
   carves from its run list first (largest run first), then maps a new
   chunk: a run's pages fault back in as they are carved. A chunk never
   goes back to the OS by unmapping (addresses stay valid for the leak
   check and the heap profile), only by advice.

Released bytes are counted for /debug/sched (0533's follow-up line,
`heap_released`), and `XO_HEAPSTATS` reports the run list.

The proving test: a program allocates a burst of 64 MB of 128 and 1,024
byte blocks, frees them all, and waits three SCAV_EVERY periods; its
resident set (Linux /proc/self/status VmRSS) returns to within 8 MB of
its value before the burst. It fails today (the burst stays resident).
Then the sql stress run on host 2 is the evidence: RSS at 30 minutes
within 10% of the 5 minute sample, the stress's own pass rule.

Stages: (1) `XO_HEAPSTATS=pages` (the measurement above), the Linux 30
minute measurement, and the test, failing; (2) the sweep and aging,
giving back without reuse (runs leave the lists: the test passes, chunks
grow on later bursts); (3) the run list and its reuse (chunk count flat
across bursts). Each stage carries an `--ab` on host 1 of the stress, the
HTTP server and xo's self build: the sweep runs on idle workers only, so
the busy paths should not move, and a regression past 1.02 stops the
stage.

## Consequences

- Idle programs return most of their small-class free memory: on the
  measured run about 50 of 56 MB with 4 KiB pages (Linux); darwin's 16 KiB
  pages reach 74%.
- A sweep costs a sort of the free blocks on an idle worker every
  SCAV_EVERY. Nothing changes on alloc or free (no counters, no prefix
  bits), so the hot paths keep their instruction counts.
- Free list order changes after a sweep (blocks near released pages
  leave): allocation addresses differ, never results.
- A run carved again faults its pages back in: a program bursting every
  period pays page faults, which the two-pass aging limits as 0533's does.
- Sanitizer and --race builds keep their libc heap: no sweep, as no age.

## Alternatives

- **Per-class spans (Go's mspans).** Each class carves its own span of
  pages with a free count, and a span whose count reaches zero goes back.
  The general answer, but a rewrite of carve, free and the magazines
  (decision 0118), with a span lookup on every free where the prefix now
  gives the class. The measurement says whole free pages are already 90%
  of the bytes under the current layout, so the sweep gets most of the
  win without it. Spans stay the next step if a workload's free blocks
  turn out interleaved with live ones (the sweep's own report shows it).
- **Per page occupancy counters in the chunk table.** Exact and cheap to
  read, but an increment on every allocation and a decrement on every
  free of the hot paths (decision 0207's inlined alloc), which every
  program pays to help idle ones.
- **Unmapping chunks.** Gives back address space too, but only when a
  whole 4 MiB chunk is free, which the measured run never has (23 chunks,
  each with live or recently freed blocks), and it invalidates addresses
  the leak check and the heap profile keep.
- **Nothing (0533's status).** Leaves sql/stress and long running servers
  2 to 3 times Go's resident set after bursts.

## Addendum 2026-11-16: stage 2 as built (Xo Lang 1)

Stage 2 (heapos.xo sweep, from age on the central pass) differs from the
text above in four ways, each smaller than the step it replaces:

- **Counts, not a sort.** Chunks are 4 MiB aligned, so the sweep counts
  each 4 KiB unit's free bytes in an array per 4 MiB window (a window
  table of 4,096 slots, an array of 1,024 counts per window touched),
  and a unit is whole free when its count is 4,096. That is one walk of
  the free blocks with no sort and no SWEEP_MAX: a pass reads every free
  block, so a unit covered by blocks a capped pass skipped is never
  missed. Its cost is three walks of the free blocks a pass, on an
  idle worker only.
- **The central claimant only, with one table.** The central pass moves
  between workers, so the previous pass's units live in the central
  table (words 180 to 183: array, count, capacity, busy flag), and only
  the worker holding the central claim sweeps: its own record's lists
  and spare magazines plus every class's central magazines, each chain
  detached under its class's lock for the walk (an allocation meanwhile
  carves from its chunk) and put back before what other threads freed
  there meanwhile. Other workers' lists hold at most two magazines a
  class and are not swept.
- **Reuse through 0630's pool.** The units given back go to the central
  pool of decision 0630 (pool_room, pool_runs; the region release's
  path), which heap_grow already carves from before mapping a chunk
  (pool_take). So stage 3's reuse comes with stage 2, and the run list
  of step 3 is that pool.
- **Linux first.** As for the region release, the pool's units are
  advised on Linux only (advise_run), so the sweep runs on Linux; darwin
  (MADV_FREE_REUSABLE, which a reuse must pair with MADV_FREE_REUSE) is
  the next step. Under transparent huge pages (Linux's dense default) a
  unit goes back only from a 2 MiB half window at least half whole
  free, age_sparse's rule for page blocks.

The proving test (internal/llvm TestRuntimeHeapIdle) also bursts again
after the idle and checks every string, so a unit given back while a
list still held one of its blocks shows as a wrong string.

## Addendum 2026-11-16 (2): idle under transparent huge pages, and the root (Xo Lang 1)

Measured on host 4 (linux/arm64, THP `always`, khugepaged
`max_ptes_none` 511, a scan every 10 s) with internal/llvm's heapidle
program (an 80 MB burst of small blocks freed, then idle):

- **khugepaged refilled what the sweep gave back.** With the sweep alone,
  VmRSS fell from 81 MB to 11.8 MB at 45 s and crept back to 47.6 MB in
  3 minutes (30.2 MB at the default, dense; 9.95 MB with XO_THP=off). A
  2 MiB range with any page still present is collapsed into a huge page,
  which faults the advised units back in. So advise_run also advises
  MADV_NOHUGEPAGE on a run it gives back, and pool_take advises
  MADV_HUGEPAGE again when the bump reuses the run (dense's rule for a
  filled range). Both in dense mode only (Linux's default): off maps every
  chunk MADV_NOHUGEPAGE already, and XO_THP=kernel promises no advice.
- **The large block cache drops on idle.** The central pass also drops
  the large block cache to CACHE_DROP_MIN (`age`), the 9 MB left above
  the start once the units stayed out.
- Result (hi-sweep6, 2026-11-16): idle VmRSS 3.7 MB at the default (was
  11.8 and climbing), 1.9 MB with XO_THP=off, from 1.2 MB before the
  burst; flat from 45 s to 195 s. The cost: 63 mappings against 47 with
  XO_THP=off, since each advised run splits its VMA.
- **The root runs the age passes too** (pool.xo root_idle): a program
  whose main fiber frees a burst on the root and then waits had no
  worker going idle after it, so no pass ran. Not once main ended (a
  pass at exit is a whole sweep for nothing), and a record's first check
  starts its clock (and its timed wake) instead of passing at once.
- Speed (host 1 `xo bench --ab`, 3 rounds): bench/perf/alloc wall 0.926
  (better), heap within noise, mapclone 0.909 within noise, jsonmap
  large 0.978 within noise; jsonmap small 1.028 worse (9.15 to 9.41 ms).
  Callgrind (linux/arm64 Docker) counts 181,957,651 instructions before
  and 181,959,636 after for jsonmap small: the 2.8% is code placement,
  not work. Max RSS unchanged on every row.

What Go does (go1.26 runtime, read 2026-11-16): the scavenger releases
with MADV_DONTNEED (sysUnusedOS; MADV_FREE only with
GODEBUG=madvdontneed=0) and gives no huge page advice on the heap. It
avoids the refill another way: findScavengeCandidate widens a release
that would break a free, unscavenged huge page to the whole 2 MiB page,
so no partly present range is left for khugepaged. Its source notes that
an eager MADV_COLLAPSE was tried and "caused performance issues in
production environments" (mgcscavenge.go).

Alternatives for the refill:
- **Release whole 2 MiB ranges only, as Go.** No advice and no VMA
  splits, but the sweep gives back 4 KiB units of 4 MiB chunks that
  stay partly in use; whole free 2 MiB ranges are the rare case after a
  burst of small blocks. Kept as the next step to measure (a sweep
  variant that widens to the 2 MiB range when its half window is whole
  free).
- **XO_THP=off by default.** 1.9 MB idle, but the dense default's
  speedups (decision 0456) go.
- **Nothing.** 47.6 MB idle after 3 minutes, most of the burst.

Limits: a solo record (a program that never spawned) is swept from its
own lists only (gotchas/age-pass-walked-solo-record.md); darwin still
waits for its MADV_FREE_REUSABLE pairing.

## Addendum 2026-11-17: the scavenger's pages under dense (Xo Lang 1)

The 2026-11-16 (2) advice covered the age pass's runs of small blocks
only. Page blocks (classes PAGE_CLASS and up, 32 KiB to 256 KiB) go back
through `scavenge`'s MADV_DONTNEED with no huge page advice, and under
XO_THP=dense khugepaged refilled them: the websocket stress's idle tail
on amd64 was 212 MB dense against 96 MB off and 101 MB kernel
(2026-11-16). internal/llvm testdata/rt/heapidlethp (about 64 MB of
40 KiB and 200 KiB strings freed, then idle) on linux/arm64 host 2
(THP `always`): VmRSS 3.7 MB at 70 s, then 13.2, 28.5 and 38.5 MB at
100, 110 and 120 s.

`scavenge` now also advises the block's whole chunk MADV_NOHUGEPAGE in
dense mode, once per chunk in a pass: the chunk, not the range, since
advice per block would split the chunk's mapping per block (Go 1.21.1
to 1.21.4 ran out of mappings that way), while a chunk's advice is
idempotent and its mapping merges again. Same program after: 2.7 MB from
70 s to 120 s. TestRuntimeHeapIdleTHP (150 s idle, arm64 and LLVM, Linux
with huge pages not `never`) holds it within 8 MB of the start.
