# std/toml changes

## 2026-10-27: perf: a small table keeps its values in a list; a value is told by its first byte

- Kind: perf
- Commit: this commit
- Found by: bench/perf/toml decode at 1.45x Go (re-measured on main d088ecee3); its CPU profile (a Map per table, put by the parser and read back key by key by the JSON writer, about 9%; value() probing for `"""`, `'''`, `true` and `false` before looking at the byte, scan.has_at 5.5%)
- Go reference: github.com/pelletier/go-toml/v2 v2.4.3 unstable/parser.go (parseVal switches on the first byte)
- Test: internal/llvm/testdata/golden/toml_basic (and internal/arm64's); twincheck bench/perf/toml; a 16-document to_json differential (tables of 9 to 11 keys, a key redefined in small and large tables and inline tables, dotted keys through a large table, arrays of tables under one, true, false and words that start like them) equal on the Go backend (std/rt's own toml), arm64 and llvm, and to the parent's std

A table's values are a list beside its keys, searched by key while the
table has at most 8 keys, with a Map from key to position built past
that; the JSON writer walks the two lists in order, where it read each
key back from the Map. value() switches on the value's first byte and
looks for the three-quote forms, `true` and `false` only after the byte
that starts them. Output is unchanged. Quiet --ab, 15 rounds, the
parent's std as A: decode wall 0.873 (0.863 to 0.893), instructions
0.889; Xo / Go about 1.27 (estimate: 1.454 x 0.873), from 1.454. The way
to Go's time is decoding straight from the parsed tree, with no JSON
text between; derive Json reads only text, so that is a compiler change
(the audit's note).

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
