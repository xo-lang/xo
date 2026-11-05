# sort1M: the gap was sort()'s second list (2026-11-04)

Lead, 2026-11-04: sort1M on the darwin/amd64 laptop (i7-9750H) was 78 ms
against Go's 74 after cfd5f7ba0; find what Go's loop does that ours does
not. Answer: the sort's loop is not the gap.

## The split

A driver that times building the input and the sort apart, 5 repetitions
per process, 3 processes each, darwin/amd64 binaries cross-built on the Mac
(Xo at origin/main 7ad3775da, Go 1.26.1 `slices.Sort`):

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

## sort() of a dead list (second push, same day)

A caller already hands its reference to the callee at the argument's last
use: `fn f(xs: List[Int]) -> List[Int] { var a = xs; a.sort_in_place(); a }`
called as `f(xs.freeze())` touched 1,042 pages, the same as no sort at all
(1,041). `sort` copied because of list_sort's own body: its Int path took
`xs` through `rt.any_of` while the merge path below still read `xs`, so the
count was 2 at pdqsort's first write. list_sort now moves `xs` into `a`
first and tests the element type on an empty list; `xs.freeze().sort()`
touches 1,041 pages (was 1,529).

micro13 sort1M_copy (`xs.freeze().sort()`), main's std (A) against this
change's (B), `xo bench --ab --no-guard --rounds=15 --warmup=1`, --release:

darwin/arm64 (the Mac):

```
wall_ms               60.06       59.76       0.364% 0.825%     0.995   0.991 to 0.999    better (n=15)
cpu_ms                59.26       58.97       0.127% 0.752%     0.995   0.991 to 0.999    too coarse (n=15)
max_rss_kb            19072       11248       0% 0.071%         0.59    0.59 to 0.59      better (n=15)
```

linux/amd64 offload host (the copy's pages cost little there):

```
wall_ms               70.13       69.05       6.705% 8.201%     0.985   0.933 to 1.031    within noise (n=15)
max_rss_kb            18624       17984       0.601% 0%         0.966   0.966 to 0.966    better (n=15)
```

i7 laptop, 11 interleaved runs each (ms): Go sort1M 73 to 79, median 75;
sort1M_copy with main's std 74 to 80, median 78; with this change 73 to 78,
median 75.

The other list builders (`map`, `filter`, `sort_by`) still build a new
list when their receiver dies at the call; decision 0508 (Proposed) covers
them.
