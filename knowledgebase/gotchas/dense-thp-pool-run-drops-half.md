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
