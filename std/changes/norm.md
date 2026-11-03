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
