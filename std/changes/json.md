# std/json changes

## 2026-10-23: perf: a derived decoder finds every field in one pass over the object

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (JSON small: find was 24% of native amd64's instructions, 83.3M; the lead's pick of the options, 2026-10-23)
- Go reference: none (encoding/json matches each member to a field as it reads it; this is the same one pass)
- Test: golden json_decode_fields (duplicates, escaped keys, any order, unknown members, a missing field) on arm64, LLVM and the Go backend; json goldens with Go parity; TestJSONTextDiff; native/json's corpus on arm64 and LLVM

A `derive Json` decoder of a struct (or an enum variant's fields) called
find once per field, and each find walked all of the object's members, so
an object of F fields cost F walks. It now calls jsontext.find_all once:
one walk over the members, each matched against the field keys, the last
member of a key winning as before. Output is unchanged. JSON small:
callgrind on linux/amd64 346.1M to 315.0M instructions (0.910); `xo bench
--ab` on darwin/arm64, 40 rounds: instructions 0.936 (native) and 0.932
(LLVM), wall time within noise on a loaded machine (spread about 30%).
The other JSON workloads, which decode no structs, are unchanged.

## 2026-10-23: perf: jsontext.find and same_key compare escaped keys in their own functions

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (JSON small on amd64 native 1.161x LLVM; callgrind jsontext.find 87.1M native vs 53.8M LLVM)
- Go reference: none (internal scan)
- Test: json std corpus and json goldens on arm64 and LLVM

The only call in find's loop was the escaped-key compare, so about ten loop
values had to survive a call and x86's five callee-saved registers left
them in stack slots. The compare is now key_eq_slow (and same_key's keys_eq_slow),
which the SSA treats as cold calls (saves only around the call). Output is
unchanged.

## 2026-10-22: security: decode and parse_value reject a byte after the value

- Kind: security
- Commit: f910b349c
- Found by: a Go gap check of the invalid JSON path (bench/results/json-invalid-go-backend-2026-10-22.md); decision by the lead, 2026-10-22
- Go reference: encoding/json (go1.26.1: RFC 8259 allows one value and white space; `{"a":1}x` is "invalid character 'x' after top-level value")
- Test: std/rt TestJsonTrailingBytes; goldens json_text, json_value, json_fast (arm64, LLVM, Go backend)

`json.decode`, `decode_str`, `parse_value` and `Request.json` accepted a
complete value followed by a byte that starts no token (`{"a":1}x`, `1 +`):
the native parser and the Go backend's token decoder read the value and
dropped the rest, which hides truncated, concatenated or corrupted input.
They now reject any byte after the value and white space with Go's text,
`invalid character 'x' after top-level value`, on every backend; a token
after the value (`1 2`, `{} {}`, `[1]]`) gets the same text where it said
`trailing data` before, so the error text of those changed too.
`json.decoder()` (a stream of values) is unchanged.

## 2026-10-22: perf: the Go backend answers invalid JSON from the scanner

- Kind: perf
- Commit: ca4d2964c
- Found by: the Go gap ranking (bench/results/go-backend-gap-2026-10-22.md: json invalid 10.2x Go)
- Go reference: encoding/json (go1.26.1: Unmarshal checks the syntax first; the error texts are the scanner's)
- Test: std/rt TestJsonSyntaxErrMatchesDecoder, TestJsonParseFastTrailingJunk

On the Go backend a decode of text that is not valid JSON read it three times
(`json.Valid`, the fast reader, then `json.Decoder.Token` building the value
up to the error). It now takes the scanner's error from one `json.Unmarshal`
check, with the token decoder's texts for an early end (`EOF`, `unexpected
EOF`); the token decoder still answers a bad object key. bench/perf/json invalid: 1,707 M to 670 M instructions (Go's
`json.Unmarshal` 205 M), `xo bench --ab` instructions 0.420 of before;
bench/results/json-invalid-go-backend-2026-10-22.md (later cuts: one
byte conversion and a type assertion, `--ab` instructions 0.908; then a copy
of encoding/json's scanner in std/rt so the text is scanned once, 2.0x to
1.15x Go, `--ab` instructions 0.631).

## 2026-10-22: perf: the Go backend's fast JSON reader reads surrogate pairs

- Kind: perf
- Commit: 41217c4a0
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
