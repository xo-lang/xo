# The rotation merge against step 2's half-buffer merge, 2026-10-25

ADR 0374's step 3 is "the merge's moves become `rt.list_swap` swaps, so no
move counts or releases a Str". Two ways to build it were measured; this
file records both, and the rotation merge loses.

## What was measured

Same probe and method as bench/results/sortip-2026-10-25.md
(`bench/perf/sortip/xo`, 2,000,000 elements, arm64, three runs a mode with
`/usr/bin/time -l`, the median, the sort's cost as the sorted run minus its
gen run). Wall is indicative only (load average 15 to 20 while the runs
went); the instruction and RSS columns decide.

| 2M elements | sort | step 2 (half buffer, counted moves) | rotation merge (swaps, no scratch) | rotation / sort | rotation / step 2 |
|---|---|---|---|---|---|
| Ints instructions | 1,062,000,000 | 1,187,000,000 | 5,483,000,000 | 5.16 | 4.62 |
| Ints extra RSS | 16.0 MB | 14.3 MB | 0.0 MB | 0.00 | 0.00 |
| Ints wall | 0.15 s | 0.17 s | 0.59 s | 3.9 | 3.5 |
| Strs instructions | 3,300,000,000 | 4,401,000,000 | 8,947,000,000 | 2.71 | 2.03 |
| Strs extra RSS | 80.0 MB | 22.7 MB | 0.0 MB | 0.00 | 0.00 |
| Strs wall | 1.02 s | 0.95 s | 1.08 s | 1.06 | 1.14 |
| UInt8 instructions | 1,042,000,000 | 1,221,000,000 | 3,589,000,000 | 3.44 | 2.94 |
| UInt8 extra RSS | 2.0 MB | 1.7 MB | 0.0 MB | 0.00 | 0.00 |
| UInt8 wall | 0.04 s | 0.06 s | 0.15 s | 3.5 | 2.5 |

## The rotation merge, as built

`merge_sort_swap` in std/native/prelude.xo (pi-deepseek-sort3, branch
sort-swap-merge): runs of 16 sorted by swap insertion, then merges that
place the middle element of the longer run with a binary search in the
other run, rotate the block between them with three reversals (swaps), and
recurse. No scratch at all: extra RSS is exactly zero, and on Strs it pays
off in wall (1.08 s against sort's 1.02, the counting it saves roughly
balances the extra swaps). It is stable (lower bound on the left run's
element, upper bound on the right run's) and it passes the module's 25
tests and the prelude corpus on arm64 and LLVM.

It loses on instructions by 2.0 to 4.6 times step 2 and 2.7 to 5.2 times
`sort`, because a rotation merge does O(n log n) swaps per level, O(n log^2
n) in all, against the half-buffer merge's O(n log n) moves. Removing the
counting (~30 instructions a Str move) does not pay for 1.3 to 2.3 extra
moves a level.

## What that leaves

The half-buffer merge with its moves as swaps needs to swap elements
between the list and a separate scratch array, which `rt.list_swap` cannot
do (one list's array), and a List cannot be extended in place to hold the
scratch (no truncate, resize or set_len in std Xo). So the shape needs one
new intrinsic, `rt.list_swap2`: the design, its soundness cases and the
alternative of a length-changing method are in ADR 0374's step 3 section.
Until then step 2 stands.
