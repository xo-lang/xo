# Bytes find_where, rfind_where, contains_where, trim_where, trim_start_where, trim_end_where, split_where and map_runes against Go's bytes package, 2026-10-24

Program: `bench/perf/bytespredicate` (one operation per iteration on a text
from a six-entry table, 200,000 iterations, the pieces summed by length) and
its Go twin `bench/perf/bytespredicate/twin.go.txt` (bytes.IndexFunc,
LastIndexFunc, ContainsFunc, TrimFunc, TrimLeftFunc, TrimRightFunc, FieldsFunc,
Map). The texts hold ASCII, multi byte runes, spaces in the middle and at both
ends, and invalid UTF-8 (which reads as U+FFFD), so the decodes, the found and
absent cases and the empty result all run. The workload is picked once,
outside the loop; both programs print the same `<workload> <checksum>` line,
which is the cross check that they do the same work (all nine match).

`xo bench --ab --rounds=10 --keep-loaded`, A the Xo build, B the Go twin, load
about 14 from other sessions on the Mac (the instruction counts are the tight
measure here; wall is noisier but agrees). The ratio column is Xo / Go, so
below 1 means Xo executes fewer instructions.

| workload | Xo, M | Go, M | Xo / Go | Xo net of index | Go net of index | net Xo / Go |
|---|---|---|---|---|---|---|
| index (the loop alone) | 31.0 | 22.4 | 1.38 | | | |
| find_where | 48.7 | 89.6 | 0.54 | 17.7 | 67.2 | 0.26 |
| rfind_where | 46.4 | 76.1 | 0.61 | 15.4 | 53.7 | 0.29 |
| contains_where | 102.1 | 227.2 | 0.45 | 71.1 | 204.8 | 0.35 |
| trim_where | 82.5 | 101.7 | 0.81 | 51.5 | 79.3 | 0.65 |
| trim_start_where | 61.5 | 73.4 | 0.84 | 30.5 | 51.0 | 0.60 |
| trim_end_where | 49.1 | 38.5 | 1.28 | 18.1 | 16.1 | 1.13 |
| split_where | 345.4 | 533.2 | 0.65 | 314.4 | 510.8 | 0.62 |
| map_runes | 423.5 | 414.2 | 1.02 | 392.5 | 391.8 | 1.00 |

Wall medians, Xo / Go: index 14.11 / 12.90 ms, find_where 18.96 / 19.41,
rfind_where 10.35 / 14.29, contains_where 15.62 / 25.29, trim_where 11.74 /
11.66, trim_start_where 10.42 / 13.23, trim_end_where 13.55 / 12.04,
split_where 35.51 / 45.39, map_runes 38.25 / 36.70. Peak RSS is 2.8 MB for Xo
on every row; Go is 4.1 to 4.2 MB, and split_where and map_runes 10.0 and
9.3 MB (Go's FieldsFunc and Map allocate the result slice and the working
byte slice).

What it says:

- Six of the eight methods execute fewer instructions than Go's, net of the
  bench's own loop: find_where 0.26, rfind_where 0.29, contains_where 0.35,
  trim_start_where 0.60, split_where 0.62, trim_where 0.65. The reason is the
  predicate call: Go passes `fn` as a func value, so IndexFunc, LastIndexFunc,
  ContainsFunc, TrimFunc and FieldsFunc call it indirectly once per rune and
  the decoder does not inline across it; Xo's closure is resolved by the
  specialization, so the walk is one loop with a direct call (the same loops
  this bench shares with the Str forms, `where_index`, `where_last_index`,
  `where_trim_left`, `where_trim_right` and `where_spans` in
  std/native/prelude.xo).
- trim_end_where is 1.13 net, the one method behind Go: its walk decodes
  backwards, and a malformed tail costs an extra decode of the sequence's
  start, where Go's DecodeLastRuneInString reads at most four bytes and never
  falls back. The texts end in ASCII and in spaces mostly, so the gap is
  small; the invalid byte at the end of one text is the case that pays.
- map_runes is level (1.00 net) and its wall is level too: both build a new
  value and copy the runs that map to themselves, Xo with
  `rt.rs_append_bytes` (the flush-the-run trick from 2026-10-23's Str
  map_runes work) and Go with append.

Next if the 1.13 on trim_end_where matters: the backward walk can stop at the
first ASCII byte of the tail without trying a start, which is what it already
does for a one byte r; the extra cost is the `start + w == end` failure path
decoding twice. Measure with `XO_PROF=cpu` before changing it; the row is
within 1.1x and the method is not on a hot path in std today.
