# Bytes find_any, rfind_any, contains_any, contains_rune, cut, cut_prefix, cut_suffix and split_after against Go's bytes package, 2026-10-24

Program: `bench/perf/bytessearch` (one operation per iteration on a text
from a six-entry table, a separator from a six-entry table or a set or rune
from a four or five entry table, 200,000 iterations, the pieces summed by
length) and its Go twin `bench/perf/bytessearch/twin.go.txt` (bytes.IndexAny,
LastIndexAny, ContainsAny, ContainsRune, Cut, CutPrefix, CutSuffix,
SplitAfter). The tables make an empty set, a one byte set, a set with a
multi byte rune, a set holding an invalid byte (U+FFFD), the ASCII fast path,
a found and an absent separator, and invalid UTF-8 in the value all run. The
workload is picked once, outside the loop; both programs print the same
`<workload> <checksum>` line, which is the cross check that they do the same
work (all nine match).

`xo bench --ab --rounds=9 --keep-loaded --no-guard --ops=200000`, load 14.0
from other sessions on the Mac, so wall time is noisy and the instruction
counts decide. A is the Xo build, B the Go twin; the ratio column below is
A/B inverted to Xo / Go, so below 1 means Xo runs fewer instructions.

| workload | Xo, M | Go, M | Xo / Go | Xo net of index | Go net of index | net Xo / Go |
|---|---|---|---|---|---|---|
| index (the loop alone) | 28.8 | 21.1 | 1.37 | | | |
| find_any | 298.6 | 151.8 | 1.97 | 269.8 | 130.7 | 2.06 |
| rfind_any | 253.8 | 174.1 | 1.46 | 225.0 | 153.0 | 1.47 |
| contains_any | 297.7 | 152.3 | 1.96 | 268.9 | 131.2 | 2.05 |
| contains_rune | 198.9 | 53.8 | 3.70 | 170.2 | 32.8 | 5.19 |
| cut | 77.6 | 49.5 | 1.57 | 48.8 | 28.4 | 1.72 |
| cut_prefix | 53.9 | 28.4 | 1.89 | 25.1 | 7.4 | 3.41 |
| cut_suffix | 54.0 | 30.0 | 1.80 | 25.2 | 9.0 | 2.82 |
| split_after | 238.0 | 201.5 | 1.18 | 209.3 | 180.4 | 1.16 |

Wall time, median per run, Xo / Go: index 1.08, find_any 1.96, rfind_any
1.47, contains_any 1.99, contains_rune 2.19, cut 1.04, cut_prefix 1.27,
cut_suffix 1.39, split_after 1.10. Wall follows the instruction counts. Peak
RSS is 2.8 MB for Xo on every row; Go is 4.1 MB, except split_after at
10.5 MB (a `[][]byte` of Count+1 entries allocated before Go knows how many
pieces there are).

What it says:

- These are new methods, so Xo has no baseline here: the numbers say what
  the first implementation costs against the port's reference, and every row
  except split_after is worse (1.2 to 5.2 times net of the loop).
- The set searches are the gap. `index_any_bytes` and `last_index_any_bytes`
  build Go's ASCII set (two 64 bit words, `ascii_set`/`in_set`) and take a
  byte loop while `chars` is all ASCII and the value is over 8 bytes, but
  four of the five sets in the table hold a rune above 0x7f ("aπ☺", ",✓"),
  so those iterations fall to the rune loop: `b.decode_rune(i)` per byte of
  the value, which is about five times Go's per-byte cost in a `for range`
  loop where DecodeRuneInString is inlined (Go nets 32.8 M for
  contains_rune, 0.16 M per 1,000 iterations).
- `cut_prefix` and `cut_suffix` net 3.4 and 2.8 times Go's: Go is a length
  test and an inlined `bytes.Equal` for a known size, Xo is a length test
  then a per-byte compare loop (`scan.eq_at` style) plus the Option.
- `cut` itself (1.72 net) is the separator search plus the same compare, and
  `split_after` (1.16 net) is close to Go because its cost is the List of
  views, which is cheap in both.

Next, in this order (each measured with `xo bench --ab` and explained with
`XO_PROF=cpu` first, per decision 0340):

1. `XO_PROF=cpu` on contains_rune and find_any at 20,000,000 iterations, to
   name the flat cost (the decode call, the rune compare, or the Str to
   Bytes conversion per call).
2. The ASCII byte in the rune loop: a value byte below 0x80 needs no decode,
   so the loop can compare it directly and only decode at 0x80 and above.
   Go's `for range` pays a decode per rune too, so this should take Xo under
   Go on ASCII text if the decode call is the cost.
3. `cut_prefix`/`cut_suffix`: a length test, then a word-at-a-time compare
   (Go's `bytes.Equal` lowers to a size-switched compare), instead of the
   byte loop.

The corpus (`internal/stdtest/testdata/corpus/bytes`) is the reference for
behavior: Go's indexAnyTests, lastIndexAnyTests, ContainsRuneTests,
cutTests, cutPrefixTests, cutSuffixTests and splitaftertests, then 7,500
seeded texts, sets and records with invalid UTF-8, on arm64, LLVM and the Go
backend; golden `bytes_search_cut` on arm64, LLVM, Go parity and wasm.
