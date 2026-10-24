# std/json changes

## 2026-10-24: perf: a decoded Duration, Time, base64 Bytes or Float is parsed once

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 3 (the JSON Time decode's +1.5% instructions: json_time_ok and json_time each parsed the text)
- Go reference: none (internal scan)
- Test: goldens base64_basic, duration_parse_edges, json_decode_fields, json_sized_ints, json_text, json_value, native_json, std_error_display and codex_gaps unchanged on arm64, LLVM (Go parity) and the amd64 Docker sample; bench/perf/jsontime and a Float, Duration, Bytes and Time record decode print the same checksums before and after

The native decoder checked each of these values with one runtime call and then
read it with a second, and both calls parsed the text (base64 decoded its
bytes twice, a Float ran parse_float_at twice). Each is now one call that
returns an Option (llrt_duration_opt, llrt_json_time_opt, llrt_b64_opt,
llrt_json_float_opt), and the decoder fails on None and reads the Some.
`xo bench --ab` (A the parent, quiet machine, 20 rounds): bench/perf/jsontime
(2000 times decoded 200 times) wall 29.01 ms to 22.25 ms, 0.767 (0.76 to
0.777), instructions 0.67; 2000 records of a Float, a Duration, base64 Bytes
and a Time decoded 100 times, wall 0.769 (0.764 to 0.777), instructions 0.738. An Int keeps its
pair: its value call is a digit loop with no checks.

## 2026-10-24: fix: the native decoder stops at 10,000 levels of nesting, as Go's does

- Kind: fix
- Commit: this commit
- Found by: Xo Lang DS-2's JSON decode corpus (the Go backend failed past 10,000 levels, native did not)
- Go reference: encoding/json scanner.go maxNestingDepth, pushParseState
- Test: golden json_depth (arrays and objects at 1, 9999, 10000, 10001 and 10002 levels, parse_value and decode), same output on arm64, LLVM and Go parity

`parse_value` and `decode` on the native backends accepted any depth. They now
fail the 10,001st open bracket with Go's text, `invalid character '[' exceeded
max depth` (`'{'` for an object), so the backends agree and hostile input is
bounded (the tape builder keeps its open containers in a list that grew with
the depth).

## 2026-10-24: perf: a decoded Time is parsed from the tape's bytes, not from a copy of its string

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (obj.reserve_slow was about 20% of a JSON Time list decode, on both backends)
- Go reference: none (internal scan)
- Test: goldens time_parse_digits and text_table unchanged on arm64 and LLVM; bench/perf/jsontime prints the same checksum before and after

The native decoder built a Str from each time string (a copy) and parsed that.
It now parses the tape's bytes in place (jsontext.time_at, text.parse_time_at)
when the string has no escapes, and falls back to the copy for one that has
(a time string is ASCII, so an escaped one is rare). `xo bench --ab` at one
revision (A the parent, quiet machine, 15 rounds, 2000 times decoded 200
times): wall 32.08 ms to 28.80 ms, 0.898 (0.885 to 0.915); instructions
644,864,162 to 654,341,788, 1.015: the check (json_time_ok) and the value
(json_time) each parse the text, which costs the instructions the copy saved. Known cost: every ok/value pair
of the decoder (time_ok and time here, b64_ok and b64_decode) does its work
twice; an Option-returning rt call would fix both (a builder item).

## 2026-10-24: feature: the native backends decode and encode the sized integers and Float32

- Kind: feature
- Commit: this commit
- Found by: the parity sweep of the Go backend against native (owner decision, 2026-10-24)
- Go reference: std/rt json.go jsonInt, jsonUint and the float32 case (unchanged)
- Test: golden json_sized_ints (a struct of Int8, Int16, Int32, UInt8, UInt16, UInt32, UInt64 and Float32: both ends of every range, one past each, a UInt64 above Int's range and its 20 and 21 digit texts, fractions and strings, Float32 rounding and overflow), same output on arm64, LLVM and Go parity

A struct with an Int8, Int16, Int32, UInt8, UInt16, UInt32, UInt64 or Float32
field failed to build on the native backends ("converting `Int` to `Int16`":
the generated decoder produced an Int and had no way to narrow it), so only
`--backend=go` could use them. The decoder now checks the type's range (the
messages are the Go backend's: "expected integer" for a signed type,
"expected non negative integer" for an unsigned one, "expected number" for a
Float32 past 2^128 - 2^103) and narrows; a UInt64 reads up to
18446744073709551615 (jsontext.uint_ok) and encodes as an unsigned number; a
Float32 encodes as the number of its value, 0.1 as 0.10000000149011612, as the
Go backend does. One difference remains: a Float32 text within 1e-16 of the
overflow threshold is rounded twice on native (to a Float, then to a Float32)
where Go rounds once, so the single decimal texts around 3.4028235677973366e38
may be accepted where Go refuses them.

## 2026-10-23: perf: find_all compares keys a word at a time, from the next expected field

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (after the one-pass decode, find_all was 53.8M of JSON small's 315.0M instructions on amd64, most of it the byte loop of keys that match)
- Go reference: none (internal scan)
- Test: golden json_decode_fields (a key within 8 bytes of the text's end added) on arm64, LLVM and the Go backend; json goldens with Go parity; TestJSONTextDiff; native/json's corpus on arm64 and LLVM

The decoder's packed keys end with 8 zero bytes, so find_all compares a key
8 bytes at a time with the last word masked (key_eq_padded): a key under 8
bytes is one compare instead of a loop of byte compares, each with bounds
checks; within 8 bytes of the text's end it compares bytes as before. Each
member's scan starts at the field after the last one matched, so members in
the fields' order match on the first try. Output is unchanged. JSON small:
callgrind linux/amd64 315.0M to 299.9M instructions (0.952; 346.1M before
the one-pass decode); `xo bench --ab` darwin/arm64, 40 rounds: wall 0.96
(0.937 to 0.99), instructions 0.954.

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
