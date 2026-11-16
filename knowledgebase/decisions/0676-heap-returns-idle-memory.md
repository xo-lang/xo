# 0676: the native heap returns idle memory to the OS

Status: Proposed (Xo Lang 1, 2026-11-16; task #91)

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
