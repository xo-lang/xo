# std/toml changes

## 2026-10-27: perf: toml's parser looks a key up once and keeps plain integers as written

- Kind: perf
- Commit: this commit
- Found by: bench/perf/toml decode CPU profile after the JSON writer change (the parser 48%)
- Go reference: github.com/pelletier/go-toml/v2 v2.4.3 unstable/parser.go (a decimal integer token is checked in place)
- Test: internal/llvm/testdata/golden/toml_basic (and internal/arm64's); TestGenGoldensNative/formats_toml; twincheck bench/perf/toml; a 20-document to_json differential (integers in every base, -0, 18 and 19 digits, the least Int, leading zeros, booleans, dates, multi-line strings, nine errors) equal on go, arm64 and llvm

A new key is added with one map lookup (its callers have already found it
absent), a value's leading `"""`, `'''`, `true` or `false` is matched in
place instead of through a view of the rest of the document, a decimal
integer of up to 18 digits is its own JSON text, and decode hands the JSON
bytes to the decoder without making a Str of them. Quiet --ab, 15 rounds,
the parent's std as A: decode wall 0.942 (0.921 to 0.955), instructions
0.94.

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
