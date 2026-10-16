# The unwinder's frame cache: two hot frames in one direct mapped slot

Status: Observed (2026-10-16, xo-fa, the whole bench set after an
unrelated std change).

## Symptom

bench/perf fault isolate on arm64 went 1.14x slower (187M to 212M
instructions) after a change that did not touch the unwinder (alloc.xo's
allocation counting), while the same workload with 300,000 iterations
instead of 20,000 got faster. A different hash in unwind.xo's frame
cache brought it back to 187M.

## Cause

The frame cache (unwind.xo lookup_frame, d8219048) was direct mapped,
64 entries keyed by pc. A code layout change put two frames of the
fault's walk into one slot, so they evicted each other and every lookup
of either missed (read_fde and the CFA program again, the cost the
cache removes). Which program and which layout collides is chance.

## Fix

Two ways a set (32 sets of 2): a lookup checks both, a miss moves the
first way's entry to the second. Measure an unwinder or cache change on
more than one build of the fault workloads (another iteration count
moves the code).
