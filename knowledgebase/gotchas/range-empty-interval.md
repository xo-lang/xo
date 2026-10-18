# Range analysis built an empty interval and the compiler faulted

Status: Observed and fixed (Xo Lang 2, 2026-10-18).

## Symptom

`xo build --backend=arm64` exited 70 with a fault inside the compiler
on programs that run fine with the Go backend:

- `fault: shift amount 64 out of range for Int at arm64/isel.xo` (safe_shl's
  `1 << (63 - khi)` with khi = -1, or -17);
- `fault: integer overflow: -(-9223372036854775808) does not fit Int at
  arm64/ranges.xo` (range_of's Sub negating `b.hi`).

TestArithProperty found it in seeds 444, 1502 and 1821 of a 2,000 seed
stabilization run. The test printed only "the arm64 build failed: <nil>":
the fault is on stderr, not a diagnostic.

## Cause

An interval with lo > hi describes a value no run computes, and every
consumer of an interval assumes lo <= hi. Three places intersected
intervals without checking:

- a checked product by a constant k is clamped to
  `[(MIN / k) * k, (MAX / k) * k]`. A product that always overflows
  saturates to [MIN, MIN] or [MAX, MAX], and the clamp empties it;
- the midpoint rule (decision 0184) intersects with `[a.lo, b.hi - 1]`;
- opt.xo bounds_at intersects the range, the derived bounds and the
  memo's branch facts, which contradict each other inside
  `if k >= 0 { if k < 0 { ... } }`.

## Fix

range_set (arm64 and LLVM ranges.xo) stores an empty interval as the full
one. The midpoint rule keeps the unrefined interval when the refinement
is empty, and bounds_at falls back to the range analysis' interval.
Golden internal/llvm/testdata/golden/range_empty_interval covers the
product and the shift. The amd64 backend uses compiler/arm64's
ranges.xo, so it gets the range_set guard too (not run on amd64).
