# A std speedup measured on its target case can slow a sibling case

Status: Observed 2026-11-19 (std/image, task #271).

## Symptom

ec79bbb58 drew std/image's destinations in place with BytesBuilder.put
(decision 0722 amended) and was measured on the copy case it targeted
(`xo bench --ab` 0.216). bench/perf/image gray, which the same change
moved onto put_be, went from 792 to 1459 ms on the linux/arm64 timing
host (B/A 1.842). The Mac twincheck before the push showed gray at 1.16
of Go and was never compared with gray's row before the change.

A second trap in finding it: the first `--ab` of the fix ran from a
branch cut before ec79bbb58, so neither side used put_be on that path
and the result was "within noise" (1.006) for a change worth 0.261.

## Cause

The change routed gray's word stores through put_be, which was an out
of line call (llrt_rs_put, never inlined because its fault call gives it
landing pads) plus raw.put per word. The copy case never stored words
that way, so its --ab could not see the cost.

## Fix

- A std change that moves several bench cases onto a new path runs
  `xo bench --ab` (host 1) on every case of that bench, not only the
  one it claims; a twincheck ratio is not an --ab.
- Before an --ab of a fix, check the branch contains the change it
  fixes (`git merge-base --is-ancestor SHA HEAD`) and rebase first.
- The fix (8-byte put_be/put_le in line, compiler/ssa call.xo) landed
  with internal/arm64 TestImageGrayDrawCost as the guard.
