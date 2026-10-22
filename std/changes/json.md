# std/json changes

## 2026-10-22: perf: the Go backend's fast JSON reader reads surrogate pairs

- Kind: perf
- Commit: (filled in after the push)
- Found by: go pprof of bench/perf/json parse on the Go backend (bench/results/go-backend-gap-2026-10-22.md)
- Go reference: encoding/json (go1.26.1: a `\u` high surrogate followed by a `\u` low one is one rune; a lone one is U+FFFD)
- Test: std/rt TestJsonParseFastSurrogatePairs, TestJsonParseFastMatchesDecoder

On the Go backend the fast JSON reader gave up at any `\u` escaped
surrogate, so a document with one emoji escape was read by
`json.Decoder.Token`. It now decodes a surrogate pair as the decoder does
and leaves a lone surrogate to the decoder: bench/perf/json parse 5,646 M
to 1,648 M instructions, `xo bench --ab` wall 0.315 of before (266 to 84 ms).

## 2026-10-21: perf: the Go backend reads JSON values without the token decoder

- Kind: perf
- Commit: ba3413a3
- Found by: go pprof of the streaming decoder (bench/results/json-vs-go-2026-10-21.md)
- Go reference: encoding/json (go1.26.1: the values and error texts are the token decoder's)
- Test: internal/stdtest/testdata/corpus/json (go, arm64); goldens json_stream, json_value, json_text; std/rt TestJsonParseFastMatchesDecoder

On the Go backend `json.decoder()`, `parse_value`, and every `Value` parse
read the text with a hand written reader (std/rt/jsonparsefast.go) and fall
back to `json.Decoder.Token` for anything it does not read the same way, so
errors are unchanged. The streaming decoder over 4 KB chunks went from 3.55
to 1.14 times Go's `json.Decoder` in instructions.

## 2026-10-21: feature: omitempty and json.Decoder

- Kind: feature
- Commit: 88b70a44
- Found by: decision 0309 (approved by the owner, 2026-10-21)
- Go reference: encoding/json `,omitempty` (TestOmitEmpty) and json.Decoder over a stream of values
- Test: internal/llvm/testdata/golden/json_omitempty, internal/llvm/testdata/golden/json_stream, test/errorcheck/wire_names

`as "key,omitempty"` (or `as ",omitempty"`) leaves an empty field out of
derive Json's output, with Go's emptiness: None, false, 0, "", and no
elements. `json.decoder()` reads a stream of values pushed in pieces
(NDJSON, concatenated objects), bounded by max_value, and `from_value`
turns a Value into a typed struct.

## 2026-10-21: test: parse_value against Go's encoding/json

- Kind: test
- Commit: b2ef88de
- Found by: differential test against Go
- Go reference: encoding/json Decoder.Token, scanner errors (go1.26.1)
- Test: std/native/json_test.xo (3,059 texts: encoding/json scanner cases and seeded random ones)

No difference found: the native reader accepts and rejects the same
texts as the Go backend's, with the same error texts.

## 2026-10-21: feature: json.Value, JSON of any shape

- Kind: feature
- Commit: 843a6de6
- Found by: survey (dynamic JSON; std/template's data, decision 0299)
- Go reference: encoding/json decoding into `any`
- Test: internal/llvm/testdata/golden/json_value (every backend)

`Value` (Null, Bool, Int, Num, Str, List, Object in key order),
`to_value`, `parse_value`, `encode`, and `get`.

## 2026-10-20: feature: field wire names and derive Json(case: ...)

- Kind: feature
- Commit: (this change)
- Found by: survey
- Go reference: encoding/json struct tags (go1.25.3), as a field's `as "key"` instead of a tag
- Test: internal/llvm/testdata/golden/json_wire_names and sql_wire_names, test/errorcheck/wire_names

A field names its JSON key with `kind as "type": Str` (a keyword or a
camelCase key no longer forces the field's name), and `derive Json(case:
camel)` (or snake, pascal, kebab) converts the other fields' keys
(decision 0277).

## 2026-10-20: fix: a newtype field of a derive Json struct

- Kind: fix
- Commit: (this change)
- Found by: user
- Go reference: none
- Test: internal/llvm/testdata/golden/json_wire_names

A newtype such as `type Email struct(Str)` as a field of a `derive Json`
struct was rejected with XO0404; it now encodes and decodes as its
underlying value on every backend, as core 2.8 says.
