# Bytes.split, split_n, fields, trim, trim_prefix and trim_suffix against Go's bytes package, 2026-10-23

Program: `bench/perf/bytessplit` (one operation per iteration on a text
from a six-entry table, with a separator or prefix from a five-entry table,
200,000 iterations, the pieces summed by length) and its Go twin
`bench/perf/bytessplit/twin.go.txt` (bytes.Split, SplitN, Fields,
TrimSpace, TrimPrefix, TrimSuffix). `xo bench --ab --rounds=9
--keep-loaded`, load 29.5 from other sessions, so wall time is noise and
the instruction counts decide; both programs print the same
`<workload> <checksum>` line, which is the cross check that they do the
same work.

| workload | Xo, M | Go, M | Xo / Go | Xo net of index | Go net of index | net Xo / Go |
|---|---|---|---|---|---|---|
| index (the loop alone) | 42.0 | 24.6 | 1.71 | | | |
| split | 267.9 | 264.2 | 1.01 | 225.9 | 239.6 | 0.94 |
| split_n | 129.6 | 137.4 | 0.94 | 87.6 | 112.8 | 0.78 |
| fields | 291.7 | 455.8 | 0.64 | 249.7 | 431.2 | 0.58 |
| trim | 56.9 | 65.8 | 0.86 | 14.9 | 41.2 | 0.36 |
| affix (trim_prefix and trim_suffix) | 69.7 | 38.7 | 1.80 | 27.7 | 14.1 | 1.96 |

Both ratio columns are Xo / Go: below 1 means Xo runs fewer instructions
than Go.

What it says:

- Net of the loop (the index row is the same loop with `len()` only), the
  new methods run at 0.36 to 0.94 of Go's instructions on split, split_n,
  fields and trim. Go's Split allocates a `[][]byte` of Count+1 entries
  before it knows how many pieces there are, and Fields allocates its
  result slice up front; the Xo List grows from empty and every piece is a
  view, so nothing is copied.
- The loop itself costs Xo 1.7 times Go (42.0 M against 24.6 M for 200,000
  iterations of two table lookups and two `len()` calls). That ratio, not
  the methods, is most of the 1.01, 0.94 and 0.86 columns.
- trim_prefix and trim_suffix are the one row where Xo is worse net (27.7 M
  against Go's 14.1 M): each call tests the length, then `scan.eq_at` in a
  loop, and Go's is a single length test and a `bytes.Equal` inlined for a
  known size. It is 2.4 ns per call either way.
- Wall time was within noise on every row except fields (Xo 32.5 ms, Go
  52.6 ms), and peak RSS was 2.8 MB for Xo against 4.2 to 10.5 MB for Go
  (the 10.5 MB is split's 200,000 pieces).

Pieces are views, and the measurement that shows it: 1,000 `trim` calls on
a 2 MiB value take 7 us in total, which no copying implementation could do
(a copy of 2 GiB would take hundreds of milliseconds). Bytes slicing lowers
to `rt.str_view` (decision 0330 step 3), so split's pieces and the trim
results are views of the value, and a piece kept alive keeps the whole
value alive.

The corpus (`internal/stdtest/testdata/corpus/bytes`) is the reference for
behavior: Go's splittests, fieldstests, trimSpaceTests and the TrimPrefix
and TrimSuffix cases of trimTests, then 4,800 seeded texts and records with
invalid UTF-8, on arm64, LLVM and the Go backend; golden `bytes_split` on
arm64, LLVM and Go parity.
