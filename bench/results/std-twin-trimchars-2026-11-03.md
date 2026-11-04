# std twin trimchars: host rows after decision 0465 step 2 (2026-11-03)

Host 1 (linux/arm64, `twin-ab3` with `xo bench --early`, n=5 and n=6 in two runs), Xo at 7101c5ed3 (views returned
without a count, decision 0465 step 2). Ratio is Xo wall over Go wall.

| workload | Go ms | Xo ms | ratio | interval | rows2 (before) |
|---|---|---|---|---|---|
| index | 5.076 | 5.445 | 1.07 | 1.06-1.09 | 1.11 |
| trim | 17.08 | 26.27 | 1.54 | 1.48-1.56 | 2.24 |
| trim_start | 14.66 | 20.39 | 1.39 | 1.29-1.45 | 1.91 |
| trim_end | 14.23 | 21.45 | 1.51 | 1.48-1.56 | 1.98 |

A second run (n=6): trim 1.59 (1.58-1.60), trim_start 1.36, trim_end 1.46. Step 2 took trim from 2.24 to about 1.55 of Go's.
Mac profile of `trim` (40 M calls): `trim_chars` 26 percent flat, `trim_start_unicode` 22, `trim_end_unicode` 13, `trim_end_ascii` 9; no single cause, the ASCII bitmap and Unicode rune loops are all spread. Not Proven (ADR 0461). The code is Lang 1's strings area (std/native/llrt/scan.xo); the next experiment is to measure `ascii_set` (a tuple of three results per call) against a fused ASCII path in `trim_chars`.
