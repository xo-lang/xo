# Bytes.replace, replace_n, to_lower, to_upper, equal_fold and repeat against Go's bytes package, 2026-10-23

Program: `bench/perf/bytestransform` (one operation per iteration on a text
from a six-entry table, with an old, a new and a count from four, four and
five entry tables, 200,000 iterations, the results summed by length) and
its Go twin `bench/perf/bytestransform/twin.go.txt` (bytes.ReplaceAll,
Replace, ToLower, ToUpper, EqualFold, Repeat). `xo bench --ab --rounds=9
--keep-loaded --metrics=instructions,wall_ms`, load 5.0 to 5.1 from other
sessions, so wall time is indicative only and the instruction counts
decide; both programs print the same `<workload> <checksum>` line on all
seven workloads, which is the cross check that they do the same work.

| workload | Xo, M | Go, M | Xo / Go | Xo net of index | Go net of index | net Xo / Go |
|---|---|---|---|---|---|---|
| index (the loop alone) | 41.4 | 23.3 | 1.78 | | | |
| replace (ReplaceAll) | 349.4 | 274.6 | 1.27 | 308.0 | 251.3 | 1.23 |
| replace_n (Replace) | 167.2 | 148.6 | 1.13 | 125.8 | 125.3 | 1.00 |
| lower (ToLower) | 272.6 | 393.9 | 0.69 | 231.2 | 370.6 | 0.62 |
| upper (ToUpper) | 302.3 | 406.4 | 0.74 | 260.9 | 383.1 | 0.68 |
| fold (EqualFold) | 75.0 | 49.9 | 1.50 | 33.6 | 26.6 | 1.26 |
| repeat | 71.8 | 79.8 | 0.90 | 30.4 | 56.5 | 0.54 |

`xo bench --ab` reports B / A where A is the Xo program and B the Go twin:
0.786 on replace's instructions is Xo running 1.27 times Go's, and 1.445 on
lower's is Xo running 0.69 times Go's. Both ratio columns above are Xo / Go:
below 1 means Xo runs fewer instructions.

What it says:

- `replace_n` is 1.00 net of the loop: the shared `replace_build` loop and
  Go's Replace do the same work, a scan and an append per match.
- `replace` is 1.23 net, because Go's ReplaceAll with a count of -1 goes
  through one generic path that counts the matches first and allocates the
  result once, while `replace_build` grows its buffer.
- `to_lower` and `to_upper` run 0.62 and 0.68 of Go's instructions: Go's
  ToLower takes a slow path that scans for non-ASCII before it decides
  nothing changes, twice per call, where `map_case` walks the value once.
- `equal_fold` is 1.26 net: Go's EqualFold compares byte runs word at a
  time on the ASCII fast path, and `fold_equal` walks runes.
- `repeat` is 0.54 net: Xo's is one `mem_copy` per copy into a buffer sized
  for the whole result.
- Wall time agrees in direction on replace, lower, upper, fold and repeat
  and is within a few percent on replace_n; index's wall spread (26.6%)
  makes its 1.19 a noise reading of a 0.7 ms difference.
- The loop itself costs Xo 1.78 times Go's instructions, so the 1.13 and
  1.50 columns are mostly the loop: `repeat` shows the effect plainly, 0.90
  all in, 0.54 net of the loop.

The corpus (`internal/stdtest/testdata/corpus/bytes`) is the reference for
behavior: Go's ReplaceTests, upperTests, lowerTests, EqualFoldTests and
RepeatTests (the two entries over Go's internal chunk limit left out, they
would be a 400 KB literal and test Go's buffering, not this API), then
1,500 random replaces, 1,200 case mappings, 1,500 folds and 1,200 repeats,
on arm64, LLVM and the Go backend; goldens `bytes_transforms` (arm64, LLVM,
Go parity, wasm) and `bytes_repeat_fault`.

Follow-up (2026-10-24): `bytestransform-fold-2026-10-24.md` takes `replace`
from 1.23 to 1.22 and `equal_fold` from 1.26 to 0.98 net of the loop.
