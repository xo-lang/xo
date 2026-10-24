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
work (all nine match, on both runs below).

`xo bench --ab --rounds=9` (first run) and `--rounds=12` (second), both with
`--keep-loaded --no-guard --ops=200000` on the Mac under a load of 14 from
other sessions, so wall time is noisy and the instruction counts decide. A is
the Xo build and B the Go twin; `--ab` prints B/A, shown here inverted as
Xo / Go, so below 1 means Xo runs fewer.

## First implementation

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

Peak RSS 2.8 MB for Xo on every row; Go 4.1 MB, except split_after at
10.5 MB (a `[][]byte` of Count+1 entries allocated before Go knows how many
pieces there are).

## After the rune-loop change

`XO_PROF=cpu` at 20,000,000 iterations put contains_rune's time in
`raw.decode` (31% flat), `prelude.bytes_decode_rune` (30% flat, 63%
cumulative) and `scan.index_rune_bytes` (20% flat): every byte of the value
was decoded, ASCII ones included, through three calls. find_any added
`scan.find_short` at 24% flat, which was `index_rune_bytes(cb, x)` called
once per rune of the value to search chars. So the loops now read a byte
below 0x80 without decoding (it is its own rune), call `raw.decode` directly,
and test a byte at 0x80 and above with `rune_in_set` (decode chars once a
rune and compare runes, as Go's `for _, c := range chars` does) instead of
IndexRune; ASCII bytes of the value still take the word set, now built from
chars' ASCII bytes only (`ascii_set_partial`).

| workload | Xo, M | Go, M | Xo / Go | Xo net of index | Go net of index | net Xo / Go (was) |
|---|---|---|---|---|---|---|
| index (the loop alone) | 28.7 | 21.1 | 1.36 | | | |
| find_any | 116.7 | 152.0 | 0.77 | 88.0 | 130.8 | 0.67 (2.06) |
| rfind_any | 147.2 | 174.3 | 0.85 | 118.5 | 153.1 | 0.77 (1.47) |
| contains_any | 115.8 | 152.2 | 0.76 | 87.1 | 131.1 | 0.66 (2.05) |
| contains_rune | 83.9 | 53.7 | 1.56 | 55.1 | 32.6 | 1.69 (5.19) |
| cut | 77.5 | 49.5 | 1.57 | 48.8 | 28.3 | 1.72 (1.72) |
| cut_prefix | 53.9 | 28.4 | 1.90 | 25.2 | 7.3 | 3.44 (3.41) |
| cut_suffix | 54.1 | 30.0 | 1.80 | 25.3 | 8.9 | 2.85 (2.82) |
| split_after | 238.1 | 202.1 | 1.18 | 209.3 | 180.9 | 1.16 (1.16) |

Wall medians, Xo / Go: index 3.66 / 3.93 ms, find_any 13.22 / 12.11,
rfind_any 12.51 / 12.46, contains_any 9.10 / 9.07, contains_rune 6.27 / 4.92,
cut 6.23 / 4.85, cut_prefix 5.64 / 4.20, cut_suffix 5.39 / 4.18, split_after
15.83 / 14.78. On wall the three set searches over a non-ASCII chars are
level with Go within the noise (the instruction counts are the tight
measure here); contains_rune and the three cuts are still behind, matching
their instruction ratios.

The Str forms are the same cores, so they moved with them (base commit
10df67f46 against the change, 12 rounds, instructions): find_any 300.4 M to
117.4 M (0.39), rfind_any 256.7 M to 149.0 M (0.58), contains_any 299.8 M to
116.5 M (0.39), contains_rune 200.1 M to 84.6 M (0.42), and the index control
29.1 M to 28.8 M (0.998, within noise).

## What is left

`XO_PROF=cpu` on the tuned build (`xo prof`, 20,000,000 iterations):

- contains_rune: 41% flat in `scan.index_rune_bytes` itself (the loop body at
  scan.xo:307), 12% `raw.decode`, 25% in the caller. The calls are gone; what
  remains is the per-rune loop, where Go's inlined range loop over a string
  carries no call at all. Go nets 32.6 M against Xo's 55.1 M.
- cut_prefix: 21% `scan.eq_at` (the compare), and 41% in drops of the bench's
  own constants (26% `drop List[List[Str]]`, 10% `drop Bytes?`, 5% `drop
  Str`): reading an element of an immutable const list in the loop costs a
  retain and a drop per read in Xo, where the Go twin reads a package level
  `[][]byte` with neither. The index control reads one list and the cuts read
  two, so part of the cut ratio is this, not the compare. Relieving it (a
  const list read that does not retain, or a `for p in SEPS` walk) is worth
  its own measurement before the compare is touched.
- find_any: the caller dominates and the loop is the rest; the set searches
  are ahead of Go now, so nothing here is urgent.

The cuts (1.7 to 3.4 net) and split_after (1.16 net) are otherwise unchanged:
`cut` is the separator search (`find_bytes`), and split_after is the List of
views.

Behavior reference: the corpus (`internal/stdtest/testdata/corpus/bytes`)
holds Go's indexAnyTests, lastIndexAnyTests, ContainsRuneTests, cutTests,
cutPrefixTests, cutSuffixTests and splitaftertests, then 7,500 seeded texts,
sets and records with invalid UTF-8, and runs on arm64, LLVM and the Go
backend; golden `bytes_search_cut` on arm64, LLVM, Go parity and wasm; the
strsearch and strwhere corpora cover the Str forms.

Behavior reference: the corpus (`internal/stdtest/testdata/corpus/bytes`)
holds Go's indexAnyTests, lastIndexAnyTests, ContainsRuneTests, cutTests,
cutPrefixTests, cutSuffixTests and splitaftertests, then 7,500 seeded texts,
sets and records with invalid UTF-8, and runs on arm64, LLVM and the Go
backend; golden `bytes_search_cut` on arm64, LLVM, Go parity and wasm; the
strsearch and strwhere corpora cover the Str forms.
