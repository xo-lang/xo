# std/json changes

## 2026-11-08: feature: `DecodeErr.go_text()`, encoding/json's text of a syntax error

- Kind: feature
- Commit: this push
- Found by: goversioninfo's FEEDBACK.md items 5 and 6 and Xo Lang 2's differential (ADR 0553)
- Go reference: encoding/json scanner.go (SyntaxError texts), decode.go checkValid (the scan of the data and the trailing space)
- Test: std/native/json_test.xo "DecodeErr.go_text"; golden json_go_text; scripts/jsonerrdiff (233 inputs against encoding/json)
- Timing: none (the text is built on the error path only)
- Shipped in: not yet released

`e.go_text()` returns Go's text where `msg` is `invalid JSON: ...`: the SyntaxError text, and `unexpected end of JSON input` for a truncated text. A text that ends inside a literal or number (`nul`, `1.`, `1e`) still gives the truncation text, where Go's scan of the data and a space names it (`in literal null (expecting 'l')`): that needs a field on `DecodeErr` (a first version had `go: Str`, e929fee96), which seeds older than the compiler's `dec_err` padding (53fda70eb) cannot build, so the field waits for the next seed (gotchas/std-error-struct-field-needs-compiler-first.md). A stream's `unexpected EOF` also maps to the truncation text; type errors keep Xo's words until `decode_go`.

## 2026-11-07: fix: the Go backend's `string` option decodes the values a duplicate key replaced as quoted

- Kind: fix
- Commit: this push
- Found by: the jsonstring corpus (ADR 0555), a mutated document with a repeated `"i"` key
- Go reference: encoding/json decode.go object() (every member is read, the last wins)
- Test: internal/stdtest corpus jsonstring (go, arm64, LLVM): `,string` encode rows, omitempty, 5 decode tables (every field kind with quoted, unquoted and malformed texts) and 400 mutated documents, all against encoding/json
- Timing: none
- Shipped in: not yet released

`rt.JsonDecQ` (the Go backend's decoder of a field with the `string` option, decision 0555) handed a duplicated key's earlier members to the plain decoder, so `{"i":"7","i":"1"}` failed where Go and the native backends decode it (the last value wins). Known differences kept, in the corpus header: Xo reads the quoted text as JSON, so the numbers only strconv accepts (`"007"`, `"2."`, hex floats) are errors where Go accepts them; a plain field given `null` is an error as an unquoted null is.

## 2026-11-07: feature: the `string` wire option, Go's `json:",string"`

- Kind: feature
- Commit: this push
- Found by: Xo Azure 1's GCP generator (about 9,500 Discovery int64 and uint64 fields generated as Str; lead, 2026-11-07)
- Go reference: encoding/json encode.go typeFields and decode.go literalStore, valueQuoted (go1.26.6)
- Test: golden json_string (arm64, LLVM, Go backend parity, wasm); test/errorcheck/wire_names
- Timing: none (a field without the option generates the code it did before)
- Shipped in: not yet released

`as "n,string"` (also `as ",string"`, `json "n,omitempty,string"`) writes a Bool, integer, Float, Float32 or Str field, or an Option of one, as a JSON string holding its literal, and reads it back the same way (decision 0555). The encoder writes the value's literal to a text of its own and quotes it (a string literal is quoted again, null stays null); the decoder reads a string, parses its text as JSON with no space around it and decodes that as the field's type. `to_value` writes the text as a `Str`. The option on any other field type is `XO0437`; a repeated or unknown option is still `XO0422`.

## 2026-11-07: feature: `derive Json(match: fold)`, Go's case-insensitive key matching

- Kind: feature
- Commit: this push
- Found by: the goversioninfo port (its FEEDBACK.md std items 3, 4 and 6): Go's versioninfo.json files mix `"Major"` and `"major"`, and the port decoded a json.Value by hand (about 200 lines)
- Go reference: encoding/json decode.go object() and fold.go foldName (go1.26.1): an exact key first, else the first field whose key folds equal; the last selecting member wins; the first type error in document order, an UnmarshalJSON error stopping the decode
- Test: golden json_fold (arm64, LLVM, Go backend parity, wasm); test/errorcheck/json_match; internal/stdtest TestJsonFoldDiff against encoding/json (seed 1, 1,500 cases, and seed 7, 4,000 cases, equal on arm64, LLVM and the Go backend)
- Timing: xo bench --ab, linux/arm64 offload host 1, 9 rounds, a 10 field struct decoded without and with the option: exact keys 227 and 237 ns (1.044), lowercased keys 258 and 350 ns (1.357; the type without the option decodes no field there), unknown keys 254 and 361 ns (1.421); types without the option run unchanged code (decision 0550, Measured)
- Shipped in: not yet released

A type with `derive Json(match: fold)` (a struct, or an enum for its variants' fields) decodes keys as encoding/json does: each member selects the field whose key equals it, else the first declared field whose key folds equal to it (each rune through ToLower then ToUpper, so the Kelvin sign selects `k` and `ß` stays `ß`), and the last member that selects a field wins. Every selected member decodes, and the error is Go's: the first in document order, a via type's before any type error, a missing field's after both. The option is per type, not inherited. Encoding is unchanged.

## 2026-11-07: perf: the Go backend compiles the parse tree decoder once, not once per type

- Kind: perf
- Commit: this push
- Found by: the armnetwork Go backend compile memory item (lead, 2026-11-07): a symbol census of the package's archive showed 3,521 copies of rt.JsonDec[T], 9.7 MB of its machine code (notes: ~/xo-lead/notes/xo-lang-2/gomem/notes.md)
- Go reference: none (Go's encoding/json decodes through reflection, one copy)
- Test: the JSON and derive goldens on the Go backend (TestGoldenGoParity json*, derive*, sql_json*, native_json); azure armnetwork's 9 tests on the Go backend
- Timing: decode speed on the Go backend, linux/amd64 offload host, 16 alternating runs of a 200 record decode loop: decode_str median 117.5 ms against 117.5 ms; from_value (the parse tree path, rt.JsonDec per field) 200.0 ms against 199.5 ms. Compile memory of azure armnetwork's package under `xo test --backend=go` (amd64 offload host, go 1.26.0, xo's go build flags, GOGC=25, two rounds each): 9.10 and 9.10 GB, 67.5 and 68.0 s before; 8.19 and 8.06 GB, 61.0 and 61.8 s after (-11%, -9%); its 9 tests pass
- Shipped in: not yet released

std/rt's JsonDec[T] (the Go backend's decoder from a parse tree, which `json.from_value` and a derived XoJsonDec call per field) was one type switch over every decodable kind, and Go compiled the whole switch once per type it decodes (each struct type is a shape of its own). It now keeps only the commonest cases (a type with XoJsonDec, Str, Int and Bool) in the generic function and decodes every other kind in one non-generic function. Only the Go backend's build changes; decoding gives the same values and errors.

## 2026-11-06: perf: string scanning skips 32 bytes at a time

- Kind: perf
- Commit: this push
- Found by: the SIMD survey's step 3b (bench/results/simd-survey-2026-11-06.md)
- Go reference: encoding/json scans a string byte by byte against a 256 entry safe set (scanner.go, encode.go appendString); simdjson classifies 32 or 64 bytes per step; this follows simdjson's step, not Go's loop
- Test: std json's tests and the json goldens (unchanged output); bench/perf/json long_strings and enc_long outputs equal before and after
- Timing: xo bench --ab, 8 rounds per order, linux/arm64 offload host 2 (exclusive), 2026-11-07, base a22a76620 against 83cce14f9, same-position (first in round against first in round): enc_long 0.855 and 0.896 (new second), 0.94 and 0.91 (new first); long_strings 0.878 and 0.868, 1.00 and 0.91 in the swapped order (its B/B control spread up to 9%); strings, enc_strings, small, parse and invalid within 0.98 to 1.02 in both orders, like their controls (no slowdown on short strings); the run overlapped the last hour of an OFF_NO_LANE soak on that host
- Shipped in: not yet released (needs a seed that knows rt.bytes_any_stop32)

The decoder's string end search (jsontext.str_end) and the encoder's string body copy (append_body) test 32 bytes at once with rt.bytes_any_stop32 for a quote, a backslash, a control byte or a byte of 0x80 and above, and step over all 32 when none is there; the byte loop handles the rest as before.

## 2026-11-06: fix: json.indent keeps the whitespace after the value

- Kind: fix
- Commit: this push
- Found by: aws-19: json.indent dropped the trailing white space that Go's json.Indent keeps
- Go reference: encoding/json Indent (indent.go go1.26.1): "trailing space characters at the end of src are preserved and copied to dst"
- Test: std/native/json_test.xo "indent and compact (S3): Go's TestCompactAndIndent" (the Go vectors added there; the Go backend already used Go's Indent)
- Shipped in: not yet released

`json.indent(text)` now ends with the space, tab, carriage return and newline characters `text` ends with, as Go's Indent does (a text with a trailing newline gives one). `json.compact` still drops them, as Go's Compact.

## 2026-11-06: perf: a derived encoder reserves a long List's text after its first element

- Kind: perf
- Commit: this push
- Found by: decision 0520's load runs (stability/notes GET /notes, 2026-11-06: growing the list's JSON text was 73% of the bytes allocated, each size class on the way keeping a free block per request in flight)
- Go reference: encoding/json's encodeState is a bytes.Buffer reused through sync.Pool, a package level cache std does not keep (gotchas/std-port-costly-go-idioms.md); the transferable part is not to grow a large buffer through every size
- Test: internal/llvm golden json_list_presize (lists of 0 to 40 elements around the 8 element floor, uneven sizes, nested lists, escaped strings, a list after a 5,000 byte prefix, and a round trip; output as before), the json goldens and std json's tests
- Timing: ab-host bench/perf/json, 3 rounds, linux/arm64 (423f8b929 vs 414902244): enc_small wall 0.951 (0.951 to 0.975), max RSS 0.98; enc_strings wall 0.893 (0.888 to 0.945), max RSS 1.001 (runs of 5 to 8 ms)
- Shipped in: not yet released

The encoder `derive Json` generates for a `List[T]` field, after writing the first element, reserves room for the rest at the first element's size times the elements still due, an eighth more, plus the commas and the bracket (jsontext.reserve_list), when the list has 8 elements or more. The text is copied once there instead of doubling through every size class.

## 2026-11-06: perf: decode reserves its tape once

- Kind: perf
- Commit: this push
- Found by: the json10kx20 RSS item (bench/results/release-vs-go-v0.40.0.md:612)
- Go reference: encoding/json decodes without a tape and reuses its scanner and encodeState through sync.Pool, a package level cache std does not keep (gotchas/std-port-costly-go-idioms.md); the transferable part is not to grow a large buffer step by step per call
- Test: the json goldens and corpus (unchanged output)
- Timing: ab-host micro13 json10kx20, 3 rounds: linux/arm64 wall 0.827, max RSS 0.785 (15,820 to 12,424 KB); linux/amd64 wall 0.909
- Shipped in: not yet released

`json.decode` and `parse_value` reserve the parser's tape at one node (3 words) per 4 bytes of text instead of growing it from 48 words: a 700 KB document's tape is one allocation, not a chain of copies. A text denser than that still grows the tape as before.

## 2026-11-04: feature: TokenReader and JsonWriter (decision 0495)

- Kind: feature
- Commit: this push
- Found by: aws-19 (the AWS SDK port: Smithy's JSON protocols read and write a response a token at a time and need encoding/json's exact forms); owner accepted decision 0495 on 2026-11-04
- Go reference: encoding/json stream.go Decoder.Token, More, InputOffset (tokenState, tokenError), encode.go encodeState.string and floatEncoder
- Test: corpus jsontoken (arm64, LLVM, Go backend): Go's tokens, input offsets, More and errors on the streams read whole and in random pieces, the tokens written back in Go's string forms, 400 strings with and without HTML escaping, the floats; tests of skip, value, max_token, number, float and raw
- Timing: see bench/results/std-twin-jsontoken-2026-11-04.md
- Shipped in: not yet released

`json.tokens(data)` and `json.token_reader()` give the tokens of a JSON
text (`Token`: delimiters, `Key`, `Str`, `Number` as written, `Bool`,
`Null`) with Go's `Decoder.Token` state machine and error texts, `more`,
`offset`, `skip` and `value`. `json.json_writer()` writes one a call at a
time in encoding/json's string and float forms. Both backends implement
them (std/native/json.xo, std/rt/jsontoken.go).

## 2026-11-04: perf: to_value and from_value with no text

- Kind: perf
- Commit: this push
- Found by: the lead's queue after decision 0488 (XO_PROF of bench/perf/toml encode on the Mac: json.to_value 76% of it, parse_value 55%, encode_str 21%)
- Go reference: none (Go has no typed value tree; the twin is Marshal then Unmarshal into `any`)
- Test: golden json_to_value (each value structurally equal to parse_value(encode_str(v)), the old path: whole Floats as Ints, NaN and the infinities as null, UInt64 above Int's range as a Num, wire keys, omitempty, Patch, via:, newtypes, valued, external and tagged enums, json.Value fields); goldens json_value, json_via, json_tagged_enum
- Timing: bench/results/json-value-direct-2026-11-04.md (host 1, --ab n=5): jsonvalue to 0.144 (0.138 to 0.145), toml encode 0.411 (0.399 to 0.427), jsonvalue from 0.757 (0.749 to 0.764)
- Shipped in: not yet released

The native backends generate a codec per type (compiler/ssa jsonval.xo)
that walks the value as the encoder does and builds the json.Value
directly; where the text changed a value on its way back, a helper gives
the same result (a whole Float in Int's range reads as an Int). The Go
backend keeps the text round trip. from_value writes a tape in jsontext's format from the Value (strings as
they are, numbers' text) and reads it with the decoder generated for T,
as decode reads the text, so its values and errors are the same.

## 2026-11-04: perf: a tagged enum's tag is matched in place

- Kind: perf
- Commit: this push
- Found by: decision 0478 stage 4 (tagged decode 1.089 of the external form on host 1; XO_PROF: the tag read into a Str, then compared per variant)
- Go reference: none (Go has no tagged enum decoding; its two pass RawMessage form is the twin)
- Test: goldens json_tagged_enum, json_via, sql_json_tagged (arm64, LLVM, Go parity, wasm); twincheck bench/perf/jsontag
- Timing: bench/perf/jsontag on host 1 (linux/arm64), `xo bench --ab`, n=6, base 11089eb18: tagfirst wall 0.857 (0.842 to 0.889), better; external (unchanged path, control) 0.984 (0.88 to 1.006), within noise; bench/results/json-own-form-tagged-2026-11-03.md
- Shipped in: not yet released

The decoder compares the tag's string node with each variant name in
place when it is ASCII without escapes (jsontext.str_is), and makes the
Str only for the unknown variant error.

## 2026-11-03: feature: a type's own JSON form (`derive Json(via: P, to: f, from: g)`)

- Kind: feature
- Commit: this push
- Found by: decision 0478 (the 2026-10-30 backend survey: money, IDs and enums kept as strings), stage 3 of its four
- Go reference: encoding/json's `Marshaler`/`Unmarshaler` and `TextMarshaler`, typed: the encoder never sees bytes; serde's `#[serde(into, try_from)]`
- Test: golden json_via (arm64, LLVM, Go parity): a struct via Str with a failing `from`, an enum via Int, a newtype via Str with an infallible `from`, in fields, `List`, `Map` and `T?`, errors at their paths; test/errorcheck/json_via (XO0413, XO0432, XO0433)
- Timing: bench/results/json-own-form-tagged-2026-11-03.md (stage 4)
- Shipped in: not yet released

A struct, enum or newtype may travel as another Json type: encoding
calls `to` and encodes its result, decoding decodes that type at the same
path and calls `from`, whose error's Display text is the decode error's
message.

## 2026-11-03: feature: tagged enums (`derive Json(tag: "type")`, `content: "data"`)

- Kind: feature
- Commit: this push
- Found by: decision 0478 (the 2026-10-30 backend survey's chat hub), stage 2 of its four
- Go reference: none in encoding/json (Go services decode a tagged message in two passes through json.RawMessage); serde's `#[serde(tag)]` and `#[serde(tag, content)]`
- Test: golden json_tagged_enum (arm64, LLVM, Go parity): round trips, a tag after the fields, unknown keys, missing and unknown tags, a non-string tag, a wrapped struct, adjacent tagging; test/errorcheck/wire_names (XO0411, XO0413, XO0431)
- Timing: bench/results/json-own-form-tagged-2026-11-03.md (stage 4)
- Shipped in: not yet released

An enum with `derive Json(tag: "type")` writes a variant as one object,
its name under the tag and its fields beside it; `content: "data"` puts
the payload under its own key. Decoding looks the tag up anywhere in the
object.

## 2026-11-03: feature: enum variant names on the wire (`case:` on an enum, `Variant as "name"`)

- Kind: feature
- Commit: this push
- Found by: decision 0478 (the 2026-10-30 backend survey's chat hub), stage 1 of its four
- Go reference: none (Go has no sum types); serde's `rename_all` and `rename` on enum variants
- Test: golden json_variant_names (arm64, LLVM, Go parity); test/errorcheck/wire_names (XO0411 to XO0414 on enums)
- Timing: none (the names are constants in the generated codec, as before)
- Shipped in: not yet released

`derive Json(case: snake)` on an enum converts its variant names
(`JoinRoom` to `join_room`, `HTTPError` to `http_error`) and their
fields' keys; `Chat as "chat.message"(body: Str)` names one variant.
Decoding reads only the wire names. An enum without either is
unchanged.

## 2026-11-02: fix: the Go backend's typed decode reads a duplicate key's replaced values, as in Go

- Kind: fix
- Commit: this commit
- Found by: the lead's 2-hourly main check (internal/llvm TestGenGoldensNative/json_fast): after the native fix below, arm64 and LLVM failed `{"name": 1, "name": "x"}` into a `name: Str` field and the Go backend still decoded it
- Go reference: encoding/json Unmarshal into a struct and a map (go1.26.1): an UnmarshalTypeError in a value a later duplicate key replaces is still returned
- Test: internal/gen/testdata/golden/json_fast (now `error at $.name: expected string` on every backend), TestGenGoldensNative, the json and jsonfuzz corpora on the Go backend (41 of 2,022 jsonfuzz cases failed there before)
- Timing: none (an object with no duplicate key takes the same path; one with a duplicate copies its member map once)
- Shipped in: the Go backend's decoder since the first release (9713c481f, v0.7.0) through v0.35.0; fixed in v0.36.0

std/rt's parsed object now keeps the non-null values a later member of
the same key replaced. A struct field's decode reads them before the
last value and a `Map[Str, V]` decode reads them before its members,
each result dropped, so the Go backend fails where arm64 and LLVM fail
(jsontext.find_dups and replaced).

Running the json fuzz corpus on the Go backend (it had run only on arm64
and LLVM) found two more differences, fixed here: `decode[json.Value]`
of a number out of Float's range gave Null instead of failing (Go and
native fail), and the range check skipped a value a duplicate key
replaced (now checked in document order); `compact` and `indent` failed
on such a number, where Go's Compact and Indent check the syntax only.

## 2026-11-02: fix: typed decode reads a duplicate key's replaced values, as in Go

- Kind: fix
- Commit: this commit
- Found by: the json fuzz harness's duplicate key finding (the entry below), which noted typed decode as open; the lead asked for it fixed
- Go reference: encoding/json Unmarshal into a struct and a map (go1.26.1): every member is decoded in turn, so an error in a value a later duplicate replaces is still returned
- Test: internal/stdtest/testdata/corpus/jsonfuzz/jsonfuzz_test.xo ("json typed" cases: a struct of three Option fields, a struct of one, and Map[Str, Int], against Go's answers)

`decode[T]` of a struct or a `Map[Str, V]` read only each key's last
value, so `{"a": "x", "a": 1}` decoded into a struct with `a: Int?` (or a
Map[Str, Int]) succeeded with 1, where Go fails on "x". A struct's
decoder now decodes the values a duplicate replaced before the last one
(jsontext.find_all lists them after the fields' slots; objects without a
duplicate key take the old path with one more compare), and a map
decodes every member as written (jsontext.members_all; a key keeps its
first position and its last value, as before). A replaced value that
fits still leaves the last one; unknown keys are still skipped unread.

## 2026-11-02: fix: a number out of range under a duplicate key fails decode, as in Go

- Kind: fix
- Commit: this commit
- Found by: the json fuzz harness (internal/stdtest/jsonfuzz_test.go, seeds 3 to 5)
- Go reference: encoding/json Unmarshal into `any` (go1.26.1): an UnmarshalTypeError in a value a later duplicate key replaces is still returned
- Test: internal/stdtest/testdata/corpus/jsonfuzz/jsonfuzz_test.xo (2,003 cases against Go's Unmarshal and Compact, seed 1, run by TestStdCorpora); TestJsonFuzz runs fresh seeds

`decode[json.Value]` and `parse_value` of an object whose key repeats
read only each key's last value, so a number out of Float's range in an
earlier value (`{"a": 1e400, "a": 1}`) was dropped and the decode
succeeded; Go fails it. The range check now walks an object's members as
written when it has a duplicate key (objects without one take the same
path as before), and the error's path names the first such number in
document order. Typed `decode[T]` still reads only each key's last value,
so an error in a replaced value (a number out of range, a value of the
wrong type) is dropped there; a typed fuzz mode is the next step.

## 2026-10-29: feature: json.Value as a field; encode_indent, indent, compact (S3)

- Kind: feature
- Commit: this commit
- Found by: milestone 0.35 S3 (the lead's list)
- Go reference: encoding/json RawMessage and `any` fields, MarshalIndent, Indent, Compact (go1.26.1)
- Test: std/native/json_test.xo (Go's TestCompactAndIndent cases, TestIndentErrors, a Value field's round trip with Go's RawMessage null case, a struct's MarshalIndent text)

A struct deriving Json may have a `json.Value` field (any JSON value as it
is; the checker accepts the type, the derived encoder writes its text and
the decoder reads the node, on the Go backend through XoJsonEnc and
XoJsonDec on Json_Value). `encode_indent[T](v, prefix, indent)` and
`encode_indent_str` (MarshalIndent), `indent(text, prefix, indent)` and
`compact(text)` (Indent and Compact) over the parsed tape with scalars
copied as spelled. Timing: bench/perf/jsonindent against encoding/json.

## 2026-10-29: feature: the Trusted variant of Value (decision 0408)

- Kind: feature (breaking for exhaustive matches on Value)
- Commit: this commit
- Found by: milestone 0.35 S6 (html/template)
- Go reference: none (html/template's HTML, CSS, JS, JSStr, URL, HTMLAttr, Srcset types)
- Test: std/native/json_test.xo (encode of a Trusted value, parse_value never makes one)

`Value.Trusted(kind, s)`: content a template trusts in one context, made
only by std/template's constructors; `encode` writes it as the plain
string s, `get` and `parse_value` know nothing of it.

## 2026-10-26: behavior: a number out of range says so in Go's words

- Kind: behavior
- Commit: this commit
- Found by: the owner's Intel tester on 0.32.0 (`1e400` was `at $: expected number`: it is a number, out of range)
- Go reference: encoding/json's UnmarshalTypeError, "cannot unmarshal number 1e400 into Go value of type float64"
- Test: goldens json_float_range and json_sized_ints on arm64, LLVM, wasm and Go parity (the Go backend prints the same text)

`decode` into a Float or Float32, and `parse_value`, now fail on a number
that is out of the type's range with `cannot unmarshal number 1e400 into
Float` (`into Float32` for a Float32 past its largest value), at the same
path as before. A value that is not a number at all keeps `expected
number`. The text is Go's with Xo's type name in place of `Go value of
type float64`.

## 2026-10-25: perf: a list element's error path is built only on an error

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 3 (a profile of the JSON Time list decode: obj.reserve and reserve_slow were 15% of it, the "[k]" path segment built for every element)
- Go reference: none (internal)
- Test: goldens json_decode_fields, json_decode_errors and the other json goldens unchanged on arm64 and LLVM; bench/perf/jsontime 0.68 of the instructions, bench/perf/json 0.96

The native decoder of a List or Set made the path segment `[k]` (two string
concats and an allocation) for each element before decoding it, and used it
only to name the element in a decode error. It is now made in the error branch;
error messages are the same (`at $[1].name: expected string`).

## 2026-10-24: fix: parse_value refuses a number out of Float's range on both backends

- Kind: fix
- Commit: this commit
- Found by: Xo Lang 3 from DS-2's cross-backend fuzzer (`json.parse_value("1e999")` was `Num 0` on arm64 and LLVM, `+Inf` on the Go backend; every overflowing literal such as 1e309, -1e999 or 1e400 became 0 natively, silent data corruption)
- Go reference: encoding/json (Unmarshal refuses with "cannot unmarshal number 1e999 into Go value of type float64"); `json.decode[Float]("1e999")` already refused on all three backends with "expected number"
- Test: golden json_float_range (arm64, LLVM, Go parity)

`parse_value` now fails with decode's error, `at $<path>: expected number`,
naming the first number that is out of range, e.g. `$[1]` or `$.a[1].b`.
1e308 still parses; an underflow such as 1e-999 is 0 on every backend, as
strconv.ParseFloat gives. Lead's decision, reversible.

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

## 2026-11-07: fix: a Float -0 stays -0 through to_value and the writer

- Kind: fix
- Commit: this commit
- Found by: Xo Azure 1 (json.json_writer().value(json.to_value(v)) wrote a float -0 as 0, json.encode and Go's encoding/json write -0)
- Go reference: encoding/json encodes float64 -0 as `-0`
- Test: internal/stdtest/testdata/corpus/jsontoken ("a Float -0 is written -0 by encode, to_value and the writer")
- Shipped in: not yet released

`to_value` turned every whole Float into an Int, which has no sign of zero; -0 is now kept as a Num and written `-0`. The text `-0` read into a Value is a Num too (Go's float64 keeps its sign), where it was an Int 0.

## 2026-11-07: behavior: a Map's keys are written sorted, as Go does (decision 0543)

- Kind: behavior
- Commit: this commit
- Found by: Xo Azure 1 (azsystemevents vectors: 21 of 178 spec examples differ from Go in map key order)
- Go reference: encoding/json sorts map keys when encoding a map
- Test: corpora of json, jsonencode and toml against Go; goldens that encode a Map
- Shipped in: not yet released

`json.encode`, `derive Json` struct fields of type `Map[Str, V]` and `json.to_value` write a Map's keys sorted by their bytes, not in insertion order, on every backend; the output no longer depends on how the Map was built. A `json.Value` object keeps its order (a parsed document round trips unchanged), and `Set` still writes insertion order. toml's `encode` goes through `to_value`, so its maps are sorted too. A Map read from an object and written back no longer keeps the document's key order; use a `json.Value` for that.

## 2026-11-07: perf: JsonWriter writes a string with one word scan and one escaping pass

- Kind: perf
- Commit: this commit
- Found by: the Xo-vs-Go survey (bench/results/xo-vs-go-survey-2026-11-07.md row 9: jsontoken/write 1.73x Go; write_string 70% cum)
- Go reference: encoding/json encodeState.string (go1.26.1): a safe-set walk that copies each run of plain bytes at once, `<`, `>`, `&` escaped in the same pass under SetEscapeHTML
- Test: testdata/corpus/jsontoken STRINGS (400 cases, and 160 new long strings with up to two bytes to escape at every offset, from Go's encoder with and without HTML escaping) on go, arm64 and llvm
- Timing: bench/perf/jsontoken on linux/arm64 host 1, --ab against main (7 rounds): write 0.75 (0.737 to 0.767, n=6; 103.2 to 77.4 ms; the survey's Go is 61.6 ms), read within noise (1.015, n=7; the control)

`JsonWriter.write_string` found the first byte to escape a byte at a time and, when there was one, built
the body with jsontext.append_body and then ran three `Str.replace` calls for the HTML escapes. It now finds
that byte a word at a time (jsontext.first_stop, the HTML bytes included when the writer escapes them),
writes the string as it is when there is none, and otherwise escapes from that byte in one pass
(jsontext.append_escaped). Output unchanged.

## 2026-11-07: fix: a Float32 is written with its own shortest digits, as Go does

- Kind: fix
- Commit: this commit
- Found by: lead (Xo Azure 1: 619 float32 fields in 63 modules need byte exact bodies; Float32(1.1) came out as 1.100000023841858)
- Go reference: encoding/json encodes a float32 with strconv.AppendFloat(b, f, 'f', -1, 32), or 'e' below 1e-6 and from 1e21 compared as float32s; Go 1.26 internal/strconv/ftoadbox.go dboxFtoa32 (BSD license)
- Test: internal/stdtest/testdata/corpus/jsonencode ("encode of a Float32 is Go's", "to_value of a Float32 field encodes as Go's float32 text", 27 hand picked patterns, every power of two, 400 random patterns)
- Shipped in: not yet released

`json.encode`, `derive Json` and `json.to_value` wrote a Float32 as the float64 shortest text of its value (`1.100000023841858`). They now write the shortest digits that read back as the same float32 (`1.1`), in Go's layout; reading already matched. The native encoder uses a port of Go's Dragonbox float32 path (`llrt/num.xo` dbox32), the Go backend `json.Marshal(float32)`. `to_value` of a Float32 holds the double nearest those digits, as the encoder's text read back gives.
