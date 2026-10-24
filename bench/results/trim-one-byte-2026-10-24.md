# Str.trim_chars: one pass and one view for a one byte cutset, 2026-10-24

Follow-up to the mixed-text case mapping work of the same day. The lead
asked for the next gap after the mixed rows reached 0.5 to 0.6 of Go;
`Str.trim_chars` was the largest one left in the string family
(`trimchars-2026-10-23.md`), and that table predates the const-read work,
so it was re-measured first.

## Re-measured against Go on today's main

Program `bench/perf/trimchars` against its Go twin, Xo arm64, `xo bench
--ab --rounds=9 --keep-loaded` at load 27.9, instructions exact, ratio
Xo / Go:

| workload | Xo | Go | Xo / Go | 2026-10-23 |
|---|---|---|---|---|
| index (loop only) | 38.76 M | 24.75 M | 1.566 | 2.878 |
| trim | 72.19 M | 40.90 M | 1.765 | 2.686 |
| trim_start | 57.18 M | 37.80 M | 1.513 | 2.497 |
| trim_end | 54.40 M | 37.10 M | 1.466 | 2.525 |

Net of the loop row, per call: trim 167 instructions against Go's 81
(2.06), trim_start 92 against 65 (1.41), trim_end 78 against 62 (1.27).
The gap held, so the per-call rows were the target.

## What was tried

All three shapes were measured the same way: the base worktree at
`origin/main` and the branch built by an xo from that revision, both
programs from the same `main.xo`, `xo bench --ab --rounds=9`, A the base.
Ratio B / A, so under 1 is better.

| shape | trim | trim_start | trim_end |
|---|---|---|---|
| 1. both ends in one pass, every ASCII path inlined in trim_chars | 0.939 | 1.046 | 1.000 |
| 2. as 1, and the loops inlined into trim_start_chars and trim_end_chars, the four helpers deleted | 0.944 | 1.053 | 1.083 |
| 3. one pass and one view for the one byte path only, the shared helpers kept for the rest | 0.917 | 1.000 | 0.999 |

Shapes 1 and 2 made `trim_start_chars` and `trim_end_chars` worse
although their source did not change: with the shared helpers' bodies
also inlined in `trim_chars`, the inliner's decisions for
`trim_start_chars` and `trim_end_chars` moved (shape 2, which deleted the
helpers and inlined their loops, was worse again). Shape 3 keeps
`trim_chars` small and leaves the helpers to the other two trims, and is
what landed.

## What shape 3 is worth

| workload | base | now | now / base |
|---|---|---|---|
| trim | 69,394,621 | 63,665,801 | 0.917 (0.915 to 0.918) |
| trim_start | 54,847,823 | 54,822,275 | 1.000 (0.998 to 1.000) |
| trim_end | 53,837,369 | 53,794,709 | 0.999 (0.997 to 1.000) |
| index | 36,662,837 | 36,659,915 | 1.000 (0.994 to 1.005) |

These are instruction counts; the machine was loaded (27.9), so wall is
not claimed. Per call trim is now 135 instructions against Go's 81
(1.67, from 2.06). The pair was measured with both sides from the same
`main.xo` built by xos of one revision.

## What is left

- The per-call remainder is mostly the view itself: the profile of a
  larger loop (`drop Str` 31% of samples) charges the temporary view's
  reference count and drop, which Go's string slice header does not have.
  That is a property of the Str representation, not of the trim.
- `ascii_set` is rebuilt on every call for a multi byte ASCII cutset
  (2 of the 7 in this benchmark). Go does the same (makeASCIISet per
  call), so it is parity work, not a gap.
- `trim_start_chars` and `trim_end_chars` still sit at 1.27 and 1.41 per
  call; the same one-pass change does not apply (one end only), and
  shape 2 showed that inlining their helpers makes them worse.
