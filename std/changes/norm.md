# std/norm changes

## 2026-11-03: feature: std/norm, Unicode normalization forms

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md row 12: Unicode normalization (NFC) and IDNA); decision 0476 (std/norm Accepted, owner 2026-11-03)
- Go reference: golang.org/x/text v0.43.0 unicode/norm (Unicode 15.0.0 tables), BSD-style license
- Test: internal/stdtest/testdata/corpus/norm (every rune a form changes, 700 seeded strings of decomposable runes, combining marks of every class, Hangul and ASCII, with `is_normalized`, against x/text; known cases) on arm64, LLVM and the Go backend; bench/perf/norm twincheck against x/text
- Cost (decision 0476 asked for it): a program that imports std/norm and calls `normalize`, against one that prints the same string without it, built with `xo build --release` for darwin/arm64: binary 227 KB to 519 KB (+292 KB), `__text` 135,552 to 222,300 bytes (+86,748), `__const` 7,011 to 206,755 bytes (+199,744). The tables are 8.5 thousand runes of decompositions as `Int` lists, 8 bytes each; a 4 byte element list or a packed table would halve the data.
- Timing: bench/results/std-twin-rows12-2026-11-03.md: NFC 5.67, NFD 2.09, NFKC 3.84, ASCII 1.17 of x/text's wall, peak memory 1.6 to 2.0; not Proven, a quick check scan and table lookup are next

`norm.Form`, `norm.normalize`, `norm.is_normalized`, `norm.unicode_version` (spec/std.md 60).

## 2026-11-03: perf: normalize copies the text that needs nothing and works only on the runs that do

- Kind: perf
- Commit: this commit
- Test: the corpus above, unchanged (arm64, LLVM, Go backend)
- Timing: Mac iteration (not evidence), bench/perf/norm: NFC 2.3, NFD 1.4, NFKC 1.7 of x/text's time, from 5.7, 2.1, 3.8 on host 1; host rows in bench/results/std-twin-rows16-2026-11-03.md: NFC 1.64, NFD 1.13, NFKC 1.11 of x/text

Runes below 0xC0 (0xA0 for the compatibility forms) are their own normalization and never combine with a rune before them, so `normalize` copies them as slices and runs decomposition, reordering and composition only on each run of runes from there up plus the one starter before it; a rune below 0x300 skips the composition pair lookup; the result is built in a `BytesBuilder` with `write_rune`.

## 2026-11-03: perf: the tables are byte records with block indexes, and normalize allocates less

- Kind: perf
- Commit: this commit
- Test: the corpus above, unchanged (arm64, LLVM, Go backend)
- Cost: a program importing std/norm grows by 66,480 bytes (was 299,008): tables 58 KB of `Bytes` constants (was 200 KB of `List[Int]`), code +9.4 KB (was +87 KB); Go's x/text/unicode/norm adds 100,096 (stripped), see bench/results/std-idna-size-2026-11-03.md
- Timing: Mac iteration (not evidence) bench/perf/norm: NFC 2.2 of x/text, NFD 1.3, NFKC 1.4; host rows in bench/results/std-twin-rows16-2026-11-03.md: NFC 1.64, NFD 1.13, NFKC 1.11 of x/text

Decompositions are 7 byte records (a rune or a run of runes at one delta, then the delta or a position in a pool of varint runes) found by a binary search bounded by a 64 rune block index; composition pairs by a search bounded by an index on the first rune; the segment loop walks the bytes, one class list per run.
