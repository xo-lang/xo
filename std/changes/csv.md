# std/csv changes

## 2026-11-15: feature: Reader.field_pos and Reader.input_offset (Go's FieldPos and InputOffset)

- Kind: feature
- Commit: this push
- Found by: scripts/stdaudit gaps csv.Reader.FieldPos and csv.Reader.InputOffset (decision 0655)
- Go reference: encoding/csv reader.go FieldPos, InputOffset, readLine's offset and readRecord's fieldPositions (go1.26.11)
- Test: internal/stdtest corpus csv "reader field_pos and input_offset" (Go's positions and offsets after every read of the 2812 READ_CASES texts, from scripts/stdcorpus csvGoPosOf; go, arm64, llvm)
- Shipped in: not yet released

A Reader records each field's line and column as it parses (the slots are reused from record to record, as Go reuses its slice) and counts the bytes of every line it reads, before `\r\n` is folded. `parse` keeps no positions.

## 2026-11-14: perf: a Reader builds a quoted field with a doubled quote or a line break in one buffer

- Kind: perf
- Commit: this push
- Found by: bench/perf/csv's quoted fields with `""` (each piece of the field was a Bytes concatenation, about five allocations per field)
- Go reference: encoding/csv readRecord appends a quoted field to its recordBuffer (go1.26.1)
- Test: TestCsvFuzz; corpus csv and csvfuzz on arm64, LLVM and Go; TestOutsideBytesValidated
- Shipped in: not yet released

read_record writes the pieces of a quoted field that is not one view of its line (a doubled quote, a line break inside the quotes, a lazy quote) to one BytesBuilder and freezes it once. bench/perf/csv on linux/arm64 host 1, --ab against its parent, 9 rounds: reader 0.923 (0.89 to 0.942, n=6; a first run 0.946, 0.919 to 0.989), parse 0.905 (0.887 to 0.926, n=6). Output unchanged.

## 2026-11-14: perf: a Reader sizes each record's field list from the last record and no longer builds a separator's text per record

- Kind: perf
- Commit: this push
- Found by: a linux/arm64 XO_PROF profile of bench/perf/csv reader (field list growth 11%, the separator's text 7% of samples)
- Go reference: encoding/csv Reader reuses its fieldIndexes slice across records (go1.26.1)
- Test: TestCsvFuzz; corpus csv and csvfuzz on arm64, LLVM and Go; TestOutsideBytesValidated
- Shipped in: not yet released

read_record takes the separator's length from unicode.utf8_len where it built the separator's text each record, and starts a record's field list with the previous record's field count as capacity. bench/perf/csv on linux/arm64 host 1, --ab against its parent, 9 rounds: reader 0.873 (0.84 to 0.878, n=6), parse 0.869 (0.858 to 0.88, n=6). Output unchanged.

## 2026-11-14: perf: a Reader checks each line's UTF-8 once

- Kind: perf
- Commit: this push
- Found by: decision 0616 perf evidence for std/csv (bench/perf/csv reader 1.533 of Go on linux/arm64 host 1); 742fd0387's per field check cost it 1.175 (host 1 --ab, e170aa4a0 against 48d129239)
- Go reference: none (Go's strings hold any bytes)
- Test: corpus csv and csvfuzz on arm64, LLVM and Go (the invalid UTF-8 Reader tests among them); TestOutsideBytesValidated; the csv stress program with --check=all
- Shipped in: not yet released

read_record checks each line a Reader reads with utf8_valid once; only a record holding a line that is not UTF-8 makes its fields valid one by one (a field cut at an ASCII separator, quote or line end from valid lines is valid). bench/perf/csv reader on linux/arm64 host 1, --ab against its parent, 6 rounds: 0.913 (0.900 to 0.933).

## 2026-11-14: fix: a lazy_quotes Reader field ending inside quotes keeps its text, and the Reader's boundaries are checked

- Kind: fix
- Commit: this push
- Found by: the second reader of the redone decision 0616 review of std/csv (std/evidence/csv/review.md, G1)
- Go reference: encoding/csv readRecord's lazy end of text inside quotes (go1.26.1)
- Before: with lazy_quotes, a Reader's text that ended inside quotes on bytes that are not UTF-8 gave an empty field on native backends (`buf.to_str() ?? ""`) and U+FFFD for each bad byte on the Go backend
- Fix: that field goes through field_str as every other field does (U+FFFD for each bad byte on every backend; parse's valid text unchanged); TestOutsideBytesValidated lists native/csv.xo field_str and std/rt/csv.go's Reader M_read as boundaries, so dropping either check fails it
- Test: corpus csv "reader with lazy quotes, a file ending inside quotes on bytes that are not UTF-8, gives U+FFFD" on arm64, LLVM and the Go backend; TestOutsideBytesValidated
- Shipped in: not yet released

## 2026-11-14: security: a Reader's fields are valid UTF-8, and a long line reads in linear time

- Kind: security
- Commit: this push
- Found by: the decision 0616 security review of std/csv (std/evidence/csv/review.md F1 and F2, the second reader's for F2)
- Go reference: encoding/csv Reader.Read over bufio (reader.go readLine of go1.26.1): Go's strings may hold any bytes and bufio grows its buffer, so neither defect has a Go counterpart
- Before: F1, csv.reader cut a file's bytes into Str fields unchecked (decision 0259), so a field holding a byte that is not UTF-8 was an invalid Str: the 7-byte file `"x""\x82"` plus a line break, read and written back with csv.format, faulted on a split UTF-8 sequence (exit 70), a fault a peer's upload reaches. F2, read_line searched the whole buffered line again and copied it after every 64 KiB read, so a line with no break was quadratic: 4, 16, 64 and 128 MB took 2, 25, 363 and 1,489 ms; Mac, native
- Fix: F1, a Reader's fields go through scan.text_of after the parse, each byte of an invalid sequence U+FFFD, so error positions stay Go's byte columns (std/rt/csv.go: textsOf on the Go backend); parse's fields, cut from a Str, are unchanged. F2, a line past the buffer gathers the chunks in a builder and searches only each new one: 128 MB in 27 ms
- Test: corpus csv (scripts/stdcorpus csv.go): "reader over a file of invalid UTF-8 gives valid fields" (the reproducer's bytes and one more, the record and format of it) and "reader of lines longer than several read chunks" (a 330 KB line, with and without quotes, against parse), on arm64, LLVM and the Go backend
- Affected: csv.reader on every backend, 0.38.0 to 0.51.1
- Shipped in: not yet released

## 2026-11-03: feature: csv.reader, records read from a File one at a time

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 3: a CSV import job over a file larger than memory)
- Go reference: encoding/csv NewReader(io.Reader) and Reader.Read (reader.go) of go1.26.1
- Test: corpus csv, Go's reader_test.go cases read through a file, and a 4,000 row file past the read chunk with quoted line breaks across chunks, on arm64, LLVM and the Go backend
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`csv.reader(file, ...)` and `Reader.read()` (spec/std.md 16): the options of `parse`, the same records and errors, the file read in 64 KiB chunks.

## 2026-10-23: feature: valid_sep and valid_comment check a delimiter before it faults

- Kind: feature
- Commit: (filled in after the push)
- Found by: the fault for an invalid separator left a program whose separator
  comes from data no way to check it (owner's decision, 2026-10-23)
- Go reference: encoding/csv reader.go validDelim (go1.26.1)
- Test: internal/stdtest/testdata/corpus/csv (VALID_CASES, 27 code points, arm64
  and LLVM) and internal/llvm/testdata/golden/csv_delims (format, parse, and Go
  parity)

`valid_sep(r)` and `valid_comment(r)` are Go's validDelim, `valid_comment`
counting `'\0'` (Go's zero value, no comment) as valid. The fault for an
invalid separator or comment stays, as spec/std.md 16 documents and decision
0298 now notes, so a program checks these values instead of faulting. The
private rule `check_delims` used is now these two functions, so the rule exists
once on each backend.

## 2026-10-23: test: the differential is a generated corpus, and the module test is gone

- Kind: test
- Commit: (filled in after the push)
- Found by: the differential test was generated by a Go program that was never
  in the repo, so it could neither be regenerated nor checked for staleness
- Go reference: encoding/csv reader_test.go readTests and writer_test.go
  writeTests (Copyright 2011 The Go Authors, BSD-style license), reader.go and
  writer.go (go1.26.1)
- Test: internal/stdtest/testdata/corpus/csv (TestStdCorpora, arm64 and LLVM);
  scripts/stdcorpus csv

`std/native/csv_test.xo` held 2,661 parse cases and 400 writer cases from a Go
program that was never committed. The corpus holds those 3,061 inputs (each old
literal decodes to the value the generator holds), Go's own 60 reader and 28
writer test inputs, and 3,000 seeded random texts and records, against Go's
Reader.ReadAll and Writer: 4,721 parse cases and 1,428 writer cases, plus 473
round trips of format then parse. It regenerates with `go run ./scripts/stdcorpus
csv` and `-check` reports a stale one. The module test is deleted; the module's
own sampling stays in `internal/llvm/testdata/golden/csv_basic`.

One divergence is recorded rather than removed: `format` writes a record of one
empty field as `""`, so it reads back as a record, where Go's Writer writes an
empty line, which Go's Reader skips. The corpus asserts Xo's form (`std/csv.xo`
documents it).

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
