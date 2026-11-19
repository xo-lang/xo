# XO_THP=dense: a pooled run dropped the reserved half and misplaced huge page advice

Status: Observed 2026-11-19 (task #355).

## Symptom

grpcreflect's 60 minute stress grew 7 to 10% from minute 30 to the end on
linux/arm64 host 4 (bound 5%), with the heap's chunks rising from 16 to 22
and XO_HEAPSTATS `taken` at 240 to 369 units. THP off, or dense with
dense_halves() false, stayed at 16 chunks, `taken 0`, 4 to 6%.

## Cause

Two bugs on dense's halves path (heapos.xo):

1. heap_grow (cabi.xo) calls pool_take before chunk()'s extend. A record
   whose first half was used up, its second half still reserved, took a
   pooled run instead; pool_take set H_BLIM 1, the half was dropped, and
   the record mapped a new chunk sooner.
2. When that run was used up, chunk() passed the bump end (the run's end,
   inside another chunk) to thp_filled as a filled chunk's end, which
   advised the 4 MB before it MADV_HUGEPAGE. Units the sweep had given back
   lay in that range, and khugepaged filled them in again
   (khugepaged-refills-released-pages.md).

Disabling only pool_take's own re-advise of the run did not help (7.0% on
both 60 minute runs): the misplaced advice came from thp_filled.

## Fix

pool_take returns false while can_extend(h); it marks H_BLIM BLIM_RUN, and
chunk() passes no filled end after a run. heapos_test.xo has one test for
each bug (they fail before the fix on linux/arm64).

How it was found: 60 minute stress runs with XO_HEAPSTATS=pages and the
per-minute /debug/sched lines, one dense path disabled at a time (diag
branch, never pushed). 30 minute runs cannot tell these apart (10% bound).

## Follow up: the used chunk's advice (Observed 2026-11-19)

Symptom: after the fix above, grpcreflect's 60 minute stress grew 14% on
host 2 (2% before it) and TestRuntimeHeapIdleTHP failed on host 4 and on
the amd64 host: RSS fell to about 6.4 MB idle, then AnonHugePages rose
from 0 to 6 MB at about 140 s and RSS to 12 MB.

Cause: thp_filled advised the whole used chunk MADV_HUGEPAGE when the
next chunk was mapped. That replaced advise_run's MADV_NOHUGEPAGE on the
runs the sweep had given back while the chunk filled, and khugepaged
(max_ptes_none 511) filled them in again on its next slow scan. The fix
above opens second halves instead of pool runs, so more chunks are used
up and the re-advice ran more often.

The larger cause: advise(), which the idle age pass and trim use to give
a free page block back, called MADV_DONTNEED only. A block in a second
half extend had advised MADV_HUGEPAGE kept that advice, and khugepaged
filled it in again. Before the fix above few second halves were opened,
so few blocks sat in advised memory.

Fix: advise() also advises the block's chunk MADV_NOHUGEPAGE under dense
(as scavenge does, decision 0676), and thp_filled advises only the new
chunk. TestRuntimeHeapIdleTHP on the amd64 host, KiB over its start after
150 s idle: 9440 with neither, 9468 with thp_filled's change only, 3836
with advise()'s only, 240 with both (207a53636 reversed: 3804). Tests:
heapos_test.xo "a used chunk keeps the sweep's no huge page advice on its
given back runs" and "a page block given back in an advised half leaves
no huge page advice" (smaps, no hg). Rule: every path that gives pages
back under dense also turns huge pages off for them, and no later advice
turns them on again until the memory is reused.
