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

## After the Unicode path change (d585dcc81; host 1, `--early`, n=6)

| workload | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|
| trim | 16.27 | 26.28 | 1.62 | 1.56-1.68 |
| trim_start | 14.11 | 20.0 | 1.42 | 1.37-1.45 |
| trim_end | 13.6 | 21.72 | 1.60 | 1.55-1.62 |

No gain: the ratios are within the spread of the earlier runs (1.54, 1.39, 1.51). `trim_start_unicode` and `trim_end_unicode` now use `raw.decode` and `rune_in_set`, which is cleaner but does not move the host wall, so the cost is not the rune lookup. Next: a host `perf` or XO_PROF=cpu profile of the workload on host 1 to rank the real cost (the Mac profile is flat), before changing the ASCII paths.

## Per cutset kind (host 1, `--early`, Xo at 8c14a46b9, n=6 to 9; workloads one, pair, abc, uni run `trim_chars` over the six texts with one cutset)

| workload | Go ms | Xo ms | ratio | interval |
|---|---|---|---|---|
| one (" ") | 9.564 | 10.57 | 1.105 | 1.073-1.192 |
| pair (" \t") | 28.2 | 25.77 | 0.914 | 0.848-0.987 |
| abc | 29.68 | 26.21 | 0.883 | 0.867-1.003 |
| uni ("é✓") | 30.05 | 55.25 | 1.839 | 1.727-1.97 |
| trim (the seven cutsets) | 15.8 | 25.54 | 1.616 | 1.599-1.649 |

The gap was the Unicode cutset path alone. Its loops decoded every rune of s and walked the cutset's runes, though the bytes at a text's ends are mostly ASCII. After "scan: the Unicode trims test an ASCII byte of s against the cutset's ASCII bitmap" (host 1 A/B, n=6): uni 0.532 of before (0.449-0.598; the interval is wide, the spread of the Go side is 19 percent), about 29 ms against Go's 30. The full `trim` row follows when host 1 is free.

