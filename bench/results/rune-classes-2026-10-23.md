# Rune classes, to_title and utf8_len: the native runtime against Go, 2026-10-23

Program: `bench/perf/unicode` (workloads; checksums must agree on every
backend) and its new Go twin `bench/perf/unicode/go` (the same workloads
with Go's `unicode`, `strings`, `strconv` and `unicode/utf8`). Instructions
retired (`/usr/bin/time -l`, best of 5), arm64 native and an LLVM -O1 build
of the same program. The machine was loaded (the lead's gate ran
alongside), so wall time is noise and the instruction counts decide.

Every workload's checksum is identical on arm64, llvm and Go, which is the
cross check that the three programs do the same work.

| workload | arm64 | llvm | Go | arm64 / Go | llvm / Go |
|---|---|---|---|---|---|
| classes (all 13) | 857 M | 693 M | 1,372 M | 0.62 | 0.51 |
| latin1 (classes of every Latin-1 rune) | 38.4 M | 30.4 M | 42.2 M | 0.91 | 0.72 |
| title (Rune.to_title) | 121 M | 102 M | 200 M | 0.61 | 0.51 |
| utf8len (Rune.utf8_len) | 51.7 M | 50.9 M | 53.8 M | 0.96 | 0.95 |
| classify (4 classes) | 247 M | 200 M | 478 M | 0.52 | 0.42 |
| runecase (to_lower, to_upper) | 204 M | 220 M | 333 M | 0.61 | 0.66 |
| lower (Str.to_lower, ASCII) | 920 M | 653 M | 2,699 M | 0.34 | 0.24 |
| upper (Str.to_upper, ASCII) | 1,059 M | 735 M | 3,247 M | 0.33 | 0.23 |
| lower_u (Str.to_lower, mixed) | 4,810 M | 4,864 M | 7,564 M | 0.64 | 0.64 |
| upper_u (Str.to_upper, mixed) | 4,770 M | 4,748 M | 7,580 M | 0.63 | 0.63 |
| eqlower (Str case compare) | 12,098 M | 12,619 M | 21,406 M | 0.57 | 0.59 |
| quote (Str.quote) | 140 M | 135 M | 297 M | 0.47 | 0.46 |

What it says:

- Classes and case mapping (the new code, and classify/runecase from the
  runtime pilot) run at 0.4 to 0.7 of Go's instructions. Go's
  `strings.ToLower`, `ToUpper` and `strconv.Quote` allocate and copy; the
  runtime's return views or reuse in place, which is most of the gap in
  the Str rows.
- The Latin-1 rows, the case the new layout is chosen for (decision
  0180's bitmap instead of Go's 256 byte properties table), are at or
  under Go: 38.4 M against 42.2 M on arm64 (0.91) and 30.4 M on LLVM
  (0.72). One bitmap byte test covers U+0080 to U+07FF; the ASCII arm
  before it is a few compares.
- `utf8_len` is four compares in both languages (0.96), as expected.

The per-class counts of every plane are checked against Go's own
`unicode` package by `internal/stdtest/testdata/corpus/runes` (generated
by `scripts/stdcorpus runes`), and by the `unicode_table` golden on LLVM,
arm64, the Go backend and wasm.

Reproduce:

```
bench/perf/unicode/measure.sh "$PWD/bin/xo" /tmp/u arm64 llvm
bench/perf/unicode/measurego.sh /tmp/u
```
