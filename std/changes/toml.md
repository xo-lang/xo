# std/toml changes

## 2026-10-27: perf: toml writes its JSON in one buffer and copies plain strings whole

- Kind: perf
- Commit: this commit
- Found by: bench/perf/toml decode at 2.23x go-toml; its CPU profile (the JSON writer 28%, the parser 48%)
- Go reference: github.com/pelletier/go-toml/v2 v2.4.3 unstable/parser.go parseBasicString (a string with no escape is a slice of the input)
- Test: internal/llvm/testdata/golden/toml_basic (and internal/arm64's); TestGenGoldensNative/formats_toml; twincheck bench/perf/toml; a 14-document to_json differential (escapes, quoted keys, inline tables, arrays of tables, nine error messages) equal on go, arm64 and llvm

The JSON text decode builds is written into one BytesBuilder (it built a
list of parts per table and array and joined them); a string with nothing
to escape is written whole, a basic string with no escape or control byte
is a slice of the document, and the dotted key a duplicate-key error
names is joined only when there is an error. Quiet --ab, 15 rounds, the
parent's std as A: decode wall 0.709 (0.703 to 0.714), instructions 0.745.
