# time format: layout text each call, Mutex Map cache, lazy const (2026-10-22)

Measures the two variants that ADR 0332 (lazy const) estimated in
bench/results/time-layout-parse-cost-2026-10-21.md (lines 29 to 35, "not
measured"). Program: bench/perf/timeformat-cache/main.xo (600,000 formats: an
ISO layout in UTC and a long layout in America/New_York, 300,000 each), native
arm64, std at origin/main df97e04d4 (with the byte buffer Layout.format of
2026-10-22). Output is the same in all three (14,700,000 bytes).

| variant | instructions retired | against Go |
|---|---|---|
| `time.format(t, z, text)`, the layout parsed each call | 5,344 M | 5.4x |
| Mutex held `Map[Str, Layout]`, a read lock per call (parse on a miss) | 1,995 M | 2.0x |
| `const ISO: Layout = layout(...)` (lazy const, decision 0332) | 1,710 M | 1.7x |
| Go `time.Format` (the same two layouts) | 983 M | 1.0x |

`xo bench --ab --rounds=7` (B / A, 95% interval):

- cache against text each call: wall 0.263 (0.239 to 0.276), instructions
  0.373 (0.373 to 0.373)
- lazy const against cache: wall 0.822 (0.775 to 0.880), instructions 0.856
  (0.856 to 0.857)

So the estimates in the 2026-10-21 file held: the cache is 2.7x fewer
instructions than parsing each call (estimated 2.5x) and the lazy const a
further 14% (the Mutex read lock and the Map lookup, about 480 instructions
per call, against the const's one `rt.const_once` check), for a total 3.1x.

Reproduce: `cd bench/perf/timeformat-cache && XO=... OUT=<scratch dir> ./ab.sh`
(the Go figure is a Go twin of the loop with time.Format; not committed).
