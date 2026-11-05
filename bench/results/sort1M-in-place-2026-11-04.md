# sort1M: the gap was sort()'s second list (2026-11-04)

Lead, 2026-11-04: sort1M on the darwin/amd64 laptop (i7-9750H) was 78 ms
against Go's 74 after cfd5f7ba0; find what Go's loop does that ours does
not. Answer: the sort's loop is not the gap.

## The split

A driver that times building the input and the sort apart, 5 repetitions
per process, 3 processes each, darwin/amd64 binaries cross-built on the Mac
(Xo at origin/main 65fe77aee, Go 1.26.1 `slices.Sort`):

| i7 laptop, ms | Xo | Go |
| --- | --- | --- |
| sort, median of 15 | 68.0 | 69.5 |
| sort, first repetition only (8 processes) | 70.2 to 86.9, median 72 | 67.6 to 72.6, median 69 |

The sort itself runs at Go's speed once its memory is warm. The first
repetition, which is what micro13 times, pays for Xo's extra memory:
`xs.freeze().sort()` returns a new sorted list (spec std.md `sort`), so the
program touches a second 8 MB array that Go's in-place `slices.Sort`
never makes. Page faults of the whole program on the arm64 Mac
(`/usr/bin/time -l`):

| program | page reclaims | max RSS |
| --- | --- | --- |
| Go micro13 sort1M | 1,003 | 13.2 MB |
| Xo: build and freeze only | 1,040 | 11.5 MB |
| Xo micro13 sort1M (`freeze().sort()`) | 1,529 | 19.6 MB |
| Xo `var l = xs.freeze(); l.sort_in_place()`, this change | 1,043 | 11.5 MB |

## The change

`sort_in_place` is the operation that matches `slices.Sort`, but for Ints it
ran the stable merge with a compare closure. It now runs list_sort_int's
pdqsort (the port of Go's pdqsortOrdered that `sort` of a `List[Int]`
already used) in the receiver's own array.

linux/amd64 offload host (Ryzen 9 9955HX), `xo bench --ab --no-guard
--rounds=15 --warmup=1`, micro13 sort1M written with `sort_in_place`, A built
with main's std (the merge), B with this change's, load 0.04:

```
wall_ms               101.5       69.17       2.596% 4.796%     0.681   0.674 to 0.702    better (n=15)
cpu_ms                98.56       67.62       3.683% 3.537%     0.686   0.671 to 0.703    too coarse (n=15)
max_rss_kb            18228       18020       0% 0%             0.989   0.989 to 0.989    better (n=15)
```

## The twin

bench/perf/micro13's Xo sort1M now runs `sort_in_place` (Lead, 2026-11-04:
the same work as Go's `slices.Sort`). Its old form stays as sort1M_copy in
both twins for the next release-vs-Go report, so the published row is shown
beside the new one and does not move silently; drop sort1M_copy after that
report.

i7 laptop, micro13 sort1M as published (ms, 9 runs each, interleaved):

| binary | runs | median |
| --- | --- | --- |
| Go | 73 to 76 | 75 |
| Xo, `freeze().sort()` (sort1M_copy) | 74 to 79 | 77 |
| Xo, `sort_in_place`, this change | 72 to 77 | 74 |

## Not done

`sort()` of a temporary that dies at the call (`xs.freeze().sort()`) could
sort the temporary's own array if the caller handed the callee its
reference at the argument's last use. That is a compiler ownership change
for `sort`, `map`, `filter` and the other list builders, and goes to an ADR
for the owner.
