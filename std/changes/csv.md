# std/csv changes

## 2026-10-22: perf: parse finds line ends, quotes and separators 16 bytes at a time

- Kind: perf
- Commit: cf61b9236
- Found by: XO_PROF=cpu (read_line, index_byte, the per-field UTF-8 check and slice checks were 45% of parse)
- Go reference: encoding/csv reader.go readLine, readRecord (go1.26.1)
- Test: internal/llvm/testdata/golden/csv_basic and std/native/csv_test.xo (arm64, LLVM, go)

`parse` searches with `scan.find_byte`, no longer validates each field as
UTF-8 again (a slice of a valid text at a field boundary is valid), and
returns a quoted field without a doubled quote as a slice. 20,000 rows of
six fields, parsed 10 times, native arm64: 829 M to 668 M instructions
(Go's package 734 M). Results are the same.

## 2026-10-21: perf: quoted fields are written in runs

- Kind: perf
- Commit: (filled in after the push)
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md
- Test: std module tests native/csv on arm64 and LLVM

`format` writes a quoted field in runs of plain bytes instead of a string per
rune: 20,000 rows x 10: 1,386 M to 621 M instructions (Go's encoding/csv 740
M). Output is the same.

## 2026-10-21: behavior: Go's errors and options (decision 0298)

- Kind: behavior
- Commit: 7b944931
- Found by: differential test against Go's encoding/csv
- Go reference: encoding/csv reader.go (readLine, readRecord, ParseError), writer.go (go1.26.1)
- Test: std/native/csv_test.xo (Go's records and errors on 2,661 texts with every option; Go's Writer on 400 row sets); internal/llvm/testdata/golden/csv_basic

Errors are Go's ParseError (texts, positions, `start_line`); `parse` and
`parse_maps` take `comment`, `fields`, `lazy_quotes`, and
`trim_leading_space`; `format` takes `crlf` and quotes as Go's Writer
(also `\.` and a leading Unicode space).

## 2026-10-20: fix: a final \r is a line end, as in Go's csv.Reader

- Kind: fix
- Commit: b0f96be5
- Found by: differential test against Go's encoding/csv (2,022 texts)
- Go reference: encoding/csv reader.go readLine (a trailing \r at EOF reads as \n)
- Test: std/native/csv_test.xo (Go's records on its cases and 2,000 random texts); internal/llvm/testdata/golden/csv_basic

A text ending in a lone `\r` (`"a,b\r\n\r"`, `"\r"`) gave an extra record
`[""]`; it now ends the text as `\n` would, so the empty last line is
skipped, as Go reads it. Every other record of the 2,022 texts already
matched Go; errors occur on exactly the same texts (positions follow
spec/std.md 16).
