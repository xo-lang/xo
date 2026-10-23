# 0298: std/csv follows Go's encoding/csv

Status: Accepted (owner, 2026-10-20)

## Context

std/csv (decision 0164, 2026-10-11) predates decision 0276 (std follows
Go's scope, code, and tests). A differential run against Go's csv.Reader
on 2,022 texts (2026-10-20) found one real bug (a final `\r` gave an
extra record; fixed in b0f96be5) and otherwise equal records, errors on
the same texts, but Xo's own error texts and positions and none of Go's
reader options (Comment, LazyQuotes, TrimLeadingSpace, FieldsPerRecord).

## Decision

- `CsvErr` is Go's ParseError: it gains `start_line`; `msg` is Go's
  (`bare " in non-quoted-field`, `extraneous or missing " in
  quoted-field`, `wrong number of fields`); positions and the display
  text are Go's.
- `parse` and `parse_maps` take Go's options as named parameters:
  `comment` (Comment; `'\0'` for none, Go's zero value), `fields`
  (FieldsPerRecord; -1 by default so existing programs read the same),
  `lazy_quotes`, `trim_leading_space`. `parse_maps` uses FieldsPerRecord
  0 (the header's count).
- `format` is Go's csv.Writer with `crlf` (UseCRLF), keeping one
  difference: the only, empty field of a record is quoted so the record
  reads back.
- The Go backend runs encoding/csv itself; the native backends port
  reader.go and writer.go.

## Consequences

- Programs that matched on the old messages (`" in an unquoted field`,
  `quoted field not closed`, `expected N fields as in the header`) see
  Go's; positions of unclosed quotes move to where the text ends.
- std/native/csv_test.xo holds Go's results on 2,661 texts with every
  option and Go's Writer on 400 row sets.
- The Go backend's generator gained negative integer defaults for std
  parameters (`fields: Int = -1`).
- 2026-10-23 (owner): an invalid separator or comment character stays a
  fault (`csv: invalid field or comment delimiter`, spec/std.md 16), not
  Go's `errInvalidDelim`, which is a plain error with no position and so
  has no CsvErr to return. `csv.valid_sep` and `csv.valid_comment` are
  Go's validDelim, so a program whose separator or comment comes from
  data checks them first.

## Alternatives

- Keep Xo's texts and add only the options: smaller, but not Go's
  behavior, which 0276 asks for.
- A Reader struct with fields (Go's shape): Xo's std style is functions
  with named parameters.
