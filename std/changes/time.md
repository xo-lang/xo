# std/time changes

## 2026-10-22: perf: format writes digits into one byte buffer

- Kind: perf
- Commit: this commit
- Found by: the mail Date header measurement (a Layout formatter was 13% slower than a hand written one), Xo Lead
- Go reference: time.Time.appendFormat (appendInt, appendNano)
- Test: std module tests native/time ("every format field and argument form"; 2,625 layout, zone and time combinations agreed with the old text)

`Layout.format` and `time.format` build the text in a byte buffer with the
digits written straight in (no `"${n}"` strings, no padding by
concatenation, no name lists built per call), and each item carries its
argument as a number instead of comparing it. Same text. 600,000 formats
of a Layout: 2,390 M to 1,637 M instructions (-31.5%, native arm64; Go's
955 M, so 2.5x to 1.7x Go); the Str layout form 6,232 M to 5,626 M (it
parses the layout on every call). bench/results/time-format-bytes-2026-10-22.md.

## 2026-10-22: feature: time.Layout, a layout parsed once

- Kind: feature
- Commit: 3d8c505f0
- Found by: bench/results/time-layout-parse-cost-2026-10-21.md
- Test: std module tests native/time ("a Layout formats and parses as its text")

`time.layout(text)` returns a `Layout` (faults on a bad layout; a constant
layout is checked at compile time), `Layout.format(t, z)` and
`Layout.parse(s, z)` give the results of `format` and `parse` with that text,
and `RFC3339_LAYOUT`, `RFC1123_LAYOUT`, `DATE_LAYOUT` and `TIME_LAYOUT` are the
named layouts. Held in a `const` (decision 0332) the layout is parsed once per
process on first use. std/http keeps its literal layout until a seed has
decision 0332 (the compiler's own build imports it).
The Str functions are unchanged. 600,000 formats: 6,202 M to 2,375 M
instructions (native arm64; Go's time.Format 955 M, so 6.5x to 2.5x Go);
600,000 parses: 3,291 M to 1,586 M (Go 471 M, 7.0x to 3.4x). The Go backend
gains little (5,666 M to 5,522 M format). `RFC1123_LAYOUT` prints the zone as
UTC (the `{zone}` field), unlike Go's abbreviation; HTTP dates keep a literal
GMT layout.

## 2026-10-21: perf: layout fields recognized in bytes

- Kind: perf
- Commit: (filled in after the push)
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md
- Test: std module tests native/time on arm64 and LLVM

A layout field and its argument are recognized by comparing the layout's bytes
(no Str per field name), and `format` builds its text in one StrBuilder.
300,000 x 2 formats 7,390 M to 6,206 M instructions, 300,000 parses 3,768 M to
3,300 M. Results and error texts are the same.

## 2026-10-21: perf: layout items carry an integer field code

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu (bench/results/regexp-time-csv-vs-go-2026-10-21.md)
- Test: std module tests native/time on arm64 and LLVM

`format` and `parse` switch on an integer code per layout item instead of
comparing field names. 300,000 x 2 formats 9,612 M to 7,390 M instructions,
300,000 parses 4,824 M to 3,768 M. Results and error texts are the same.

## 2026-10-21: perf: layout fields validated without a list

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu (bench/results/regexp-time-csv-vs-go-2026-10-21.md)
- Test: std module tests native/time on arm64 and LLVM

A layout field is checked without building its argument list; the list is
built only for the error text. 300,000 formats: 9,612 M to 8,725 M
instructions; parses 4,824 M to 4,382 M. Results and error texts are the same.

## 2026-10-21: fix: the sign of an offset under a minute

- Kind: fix
- Commit: dd2e401a
- Found by: differential test against Go
- Go reference: time.Time.Format's zone offset (format.go, go1.26.1)
- Test: std/native/time_test.xo (1,200 random instants in fixed zones, both layouts covering every field, against Go's Format); internal/llvm/testdata/golden/time_fixed_offsets (arm64, LLVM, and the Go backend agree)

`{offset}` and `{offset:compact}` of a zone whose offset is negative but
under a minute (-59s) showed `-00:00`; Go takes the sign of the offset in
whole minutes and shows `+00:00`. Native and the Go backend's formatter
both had it. Everything else matched Go on all 1,200 instants.
