# std/csv changes

## 2026-10-20: fix: a final \r is a line end, as in Go's csv.Reader

- Kind: fix
- Commit: (filled in after the push)
- Found by: differential test against Go's encoding/csv (2,022 texts)
- Go reference: encoding/csv reader.go readLine (a trailing \r at EOF reads as \n)
- Test: std/native/csv_test.xo (Go's records on its cases and 2,000 random texts); internal/llvm/testdata/golden/csv_basic

A text ending in a lone `\r` (`"a,b\r\n\r"`, `"\r"`) gave an extra record
`[""]`; it now ends the text as `\n` would, so the empty last line is
skipped, as Go reads it. Every other record of the 2,022 texts already
matched Go; errors occur on exactly the same texts (positions follow
spec/std.md 16).
