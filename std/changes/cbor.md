# std/cbor changes

## 2026-11-15: feature: cbor.Raw (RawMessage); fix: the reserved additional information error names the type

- Kind: feature, fix
- Commit: this push
- Found by: the stdaudit gaps of fxamacker/cbor (decision 0655): RawMessage, MarshalCBOR, UnmarshalCBOR (0527 accepted `cbor.Raw`, not built until now); porting TestMarshalRawMessageContainingMalformedCBORData showed the decoder's `for major type 0`
- Go reference: github.com/fxamacker/cbor/v2 v2.9.0 stream.go (RawMessage), valid.go (the additional information error) (MIT)
- Test: internal/llvm/testdata/golden/cbor_raw (TestRawMessage, TestMarshalRawMessageValue, the malformed data errors; arm64, LLVM); std/cbor_test.xo: decode_test.go's reserved additional information cases (failed before)
- Shipped in: not yet released
- Timing: none

A `derive Cbor` field of type `cbor.Raw` keeps its item's bytes; `marshal`
writes them back as they are (empty as null), refusing bytes that are not
one well formed item with Go's `error calling MarshalCBOR for type
cbor.RawMessage: ...`. The bytes are the preferred serialization of the
item, where Go keeps the input's own. `decode` now says `invalid additional
information 31 for type positive integer`, as Go does, not `for major type 0`.

## 2026-11-15: fix: the typed decoder's error for a built in tag with the wrong content

- Kind: fix
- Commit: this push
- Found by: mapping fxamacker's InadmissibleTagContentTypeError in the stdaudit gaps (decision 0655)
- Go reference: github.com/fxamacker/cbor/v2 v2.9.0 common.go (validBuiltinTag) (MIT)
- Test: std/cbor_test.xo: tags 0, 1, 2 and 3 over the wrong content through the generated decoder's helper (failed before: `cbor: cbor: invalid content of tag 0`)
- Shipped in: not yet released
- Timing: none

`unmarshal` of a Time or Int field holding tag 0 over a number, tag 1
over text, or tag 2 or 3 over anything but a byte string now fails with
fxamacker's text (`cbor: tag number 0 must be followed by text string,
got positive integer`) instead of `cbor: cbor: invalid content of tag 0`.

## 2026-11-15: feature: time formats (from_time_as, Value.to_time)

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of fxamacker/cbor (decision 0655): TimeRFC3339, TimeRFC3339Nano, TimeTagToAnyMode, ByteStringToTimeMode, DecTagMode, EncTagMode
- Go reference: github.com/fxamacker/cbor/v2 v2.9.0 encode.go (encodeTime), decode.go (parseToTime) (MIT)
- Test: std/cbor_test.xo: RFC 8949 Appendix A's tag 0 and tag 1 times, each TimeMode, each DecTagMode, byte strings allowed and refused, Go's error texts
- Shipped in: not yet released
- Timing: none

`from_time_as(t, format, tag)` writes a Time as epoch seconds (integer,
microsecond float or dynamic) or RFC 3339 text, optionally under tag 0 or
1. `Value.to_time(rule, bytes)` reads one back with fxamacker's tag rules
and, when asked, from a byte string of RFC 3339 text.

## 2026-11-15: feature: big integers (from_big, to_big, reject_bignum)

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of fxamacker/cbor (decision 0655): BigIntConvertMode, BigIntDecMode, BignumTagMode, IntDecConvertSignedOrBigInt
- Go reference: github.com/fxamacker/cbor/v2 v2.9.0 encode.go (encodeBigInt), decode.go and valid.go (bignum tags) (MIT)
- Test: std/cbor_test.xo: 0, 1, -1 and the 64 bit edges both ways in each convert mode, Reject's error, reject_bignum's error and another tag passing
- Shipped in: not yet released
- Timing: none

`from_big(n)` writes a std/big integer as fxamacker writes a big.Int
(major type 0 or 1 when it fits, else tag 2 or 3; `BigConvert.None`
always tags). `to_big` reads any integer or bignum back.
`DecodeOptions.reject_bignum` refuses tags 2 and 3.

## 2026-11-15: feature: encode_sorted, the stream Encoder and diagnostic_with

- Kind: feature
- Commit: this push
- Found by: the stdaudit gaps of fxamacker/cbor (decision 0655): SortLengthFirst, SortCanonical, SortCTAP2, CanonicalEncOptions, CTAP2EncOptions, Encoder.StartIndefinite*, EndIndefinite, ByteStringEncoding
- Go reference: github.com/fxamacker/cbor/v2 v2.9.0 encode.go (SortMode), stream.go (Encoder), diagnose.go (ByteStringEncoding) (MIT)
- Test: std/cbor_test.xo: TestMapSort's mixed keys in both orders, the TestIndefinite* encodings and errors, diagnose_test.go's byte string encodings (ported, THIRD_PARTY_NOTICES 12)
- Shipped in: not yet released
- Timing: none

`encode_sorted(v, KeyOrder.LengthFirst)` writes RFC 7049's Canonical
CBOR map order (shorter encoded keys first). `encoder()` gives a stream
writer whose `start_indefinite` and `end_indefinite` write indefinite
length strings, arrays and maps, checking string chunks and breaks as Go
does. `diagnostic_with(DiagBytes.Base64)` and the others write byte
strings as RFC 8610's b32, h32 and b64 forms.

## 2026-11-15: feature: decode_with and DecodeOptions (fxamacker's DecOptions limits and refusals)

- Kind: feature
- Commit: this push
- Found by: the stdaudit features line (decision 0655): MaxArrayElements, MaxMapPairs, IndefLengthForbidden, TagsForbidden, NaNDecodeForbidden, InfDecodeForbidden, SimpleValueRegistry and their errors
- Go reference: github.com/fxamacker/cbor/v2 v2.9.0 valid.go and decode.go (MIT)
- Test: internal/stdtest/testdata/corpus/cbor (scripts/stdcorpus cbor): 44 inputs under 8 option sets (each option alone and all together) against fxamacker's DecMode through internal/stdtest/cborref opts, the error texts compared (arm64, LLVM, Go)
- Timing: none

`cbor.decode_with(data, opts)` bounds array and map counts and refuses
indefinite lengths, tags, NaN, infinity or chosen simple values. The
defaults are `decode`'s. Where fxamacker's well formed pass finds a refused
item after an earlier UTF-8 or simple value error, std/cbor reports the
first in data order.

## 2026-11-15: feature: `CborErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`CborErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-07: perf: marshal of a derived type writes its encoding directly, without a Value tree

- Kind: perf
- Commit: this push
- Found by: the Xo-vs-Go survey (bench/results/xo-vs-go-survey-2026-11-07.md row 2, cbor/umarshal); decision 0559
- Go reference: github.com/fxamacker/cbor/v2's struct encoder (a field list per type, one buffer)
- Test: the cbor corpora on go, arm64 and llvm (both modes); the goldens cbor_case, cbor_enum and the new cbor_direct (keyasint, omitempty, toarray, Option, List, Map, nested, Time, Duration, enum fields; main's bytes before this change) on arm64, LLVM, Go and wasm
- Timing: bench/perf/cbor on linux/arm64 host 1, ab-host 064a0f08f against the prototype f875ed5fc (7 rounds): umarshal 0.46 (0.445 to 0.468, n=6; 6.107 to 2.809 ms, Go's survey row 4.3 ms), canon 0.982 (control, the tree encoder); __text of bench/perf/cbor 433,196 to 427,988 bytes (darwin/arm64)
- Shipped in: not yet released

`derive Cbor` generates `cbor_put__T`, which writes a struct's map (or toarray array) into the output with
its keys as constant bytes, both field orders computed at derive time, and omitted fields counted before the
head; `marshal` calls it instead of building and encoding a `cbor.Value` tree. Enum and `cbor.Value` fields
still go through the tree encoder (`put_value`). Output byte identical in both modes. One difference: the
tree encoder's depth limit no longer applies to marshal of a derived value (only a recursive type nests that
deep). New std/cbor helpers for the generated code: `put_head`, `put_int`, `put_float`, `put_text`,
`put_bytes`, `put_value`, `det_keys`.

## 2026-11-03: feature: std/cbor, RFC 8949 encode, decode, diagnostic notation and the JSON bridge

- Kind: feature
- Commit: this commit
- Found by: roadmap priority 1 (Xo Lead), decision 0468
- Go reference: none in Go's standard library; github.com/fxamacker/cbor/v2 (MIT) is the differential oracle
- Test: testdata/corpus/cbor (RFC 8949 Appendix A, malformed and edge inputs, changed copies, API tests), testdata/corpus/cborfuzz (1,500 seeded inputs from a byte level builder), `TestCborFuzz` (fresh seeds), bench/perf/cbor (twincheck)

New module. `cbor.decode`, `cbor.encode` (preferred and deterministic), `cbor.diagnostic`,
`cbor.to_json` and `cbor.from_json` over one `Value` tree. 12,000 fresh fuzz cases (four seeds)
agree with the oracle on arm64 and LLVM.

## 2026-11-05: feature: derive Cbor, cbor.marshal, cbor.unmarshal and cbor.unmarshal_prefix

- Kind: feature
- Commit: this commit
- Found by: roadmap (Xo Lead), decision 0527
- Go reference: github.com/fxamacker/cbor/v2 Marshal and Unmarshal (MIT), the differential oracle
- Test: testdata/corpus/cbor (2,000 typed cases against internal/stdtest/cborref `typed`, `typed2` and `typed3`)

Typed CBOR through a derive: `marshal[T]`, `unmarshal[T]`, `unmarshal_prefix[T]` for a struct that derives
Cbor. BREAKING: new `CborErr.Type(msg)` variant (an exhaustive `match` on `CborErr` needs the arm). New public
helpers used by the generated code: `Unm`, `unm_new`, `to_str` and the other `to_*` readers, `from_time`,
`from_duration`, `items`, `pairs`, `key_text`, `key_int`, `unknown_key`, `sorted_keys`.

## 2026-11-06: feature: derive Cbor(case:)

- Kind: feature
- Commit: this commit
- Found by: decision 0527 follow-up (Xo Lang 3)
- Go reference: none (fxamacker/cbor has no key case rule; Xo's own, as `derive Json(case:)`)
- Test: golden cbor_case (camel and kebab, a field's `as` wins, strict decode by the converted keys)

`derive Cbor(case: snake | camel | pascal | kebab)` converts every field without its own `cbor` or `as`
string, by the same rule as `derive Json` (decision 0277). An unknown case is `XO0413`.

## 2026-11-06: feature: derive Cbor on enums

- Kind: feature
- Commit: this commit
- Found by: decision 0527 follow-up (Xo Lang 3)
- Go reference: none (fxamacker/cbor has no sum types; Xo's own form, as `derive Json` writes an enum)
- Test: golden cbor_enum (round trips, a hand built map, an unknown variant, an enum in a struct and a List)

A plain enum without type parameters can `derive Cbor`: a unit variant is the text of its name, a variant with fields a one
entry map from its name to a map of its fields (a lone unnamed `from` field is the value itself). An unknown variant or a map
that is not one entry is `CborErr.Type`. New `cbor.bad_variant` helper for the generated code.

## 2026-11-06: feature: derive Cbor(case:) on enums

- Kind: feature
- Commit: this commit
- Found by: decision 0527 follow-up (Xo Lang 3)
- Go reference: none (Xo's own, as `derive Json(case:)` on an enum)
- Test: golden cbor_enum (`Mode` with `case: snake`)

On an enum, `derive Cbor(case: snake | camel | pascal | kebab)` converts the variant names on the wire (`FastPath` is
`fast_path` under snake); a variant's `as "name"` wins.

## 2026-11-06: perf: the diagnostic float digits are stripped by bytes

- Kind: perf
- Commit: this commit
- Found by: `xo vet --cost --cost-std` (XO0913, TestVetCostStd) after decision 0460
- Go reference: none
- Test: testdata/corpus/cbor, testdata/corpus/cborfuzz, cmd/xo TestVetCostStd

`diag_float` stripped the leading and trailing zeros of the digit text with a Str slice per step; it now walks the bytes and
slices once. No output change (cbor corpus 16 and cborfuzz 1,500 pass).

## 2026-11-07: perf: deterministic map order without encoding the keys

- Kind: perf
- Commit: this commit
- Found by: the Xo-vs-Go survey (bench/results/xo-vs-go-survey-2026-11-07.md row 2: cbor/umarshal 3.49x Go; the key sort and the per-key builders)
- Go reference: github.com/fxamacker/cbor/v2's deterministic (core) sort: it orders a struct's fields once by their encoded keys
- Test: testdata/corpus/cbor and cborfuzz on go, arm64 and llvm; TestCborFuzz (2,000 cases against the oracle, arm64 and llvm); the bench program's marshal and canon output, a 40-key mixed map and a Map field byte identical to main's
- Timing: bench/perf/cbor on linux/arm64 host 1, --ab against main (7 rounds, n=6): umarshal 0.442 (0.433 to 0.45; 14.12 to 6.24 ms; the survey's Go is 4.3 ms), canon 0.353 (0.346 to 0.36)

A deterministic map whose keys are all text strings or unsigned integers (every derived struct's) is ordered
by comparing the keys themselves: an integer before text, integers by value, text by length and then bytes,
which is the bytewise order of their encodings (RFC 8949 4.2.1). Keys already in order are written as they
come, and up to 16 keys are sorted by insertion. Other maps (byte string, negative, float or nested keys, or
more than 16 unsorted keys) keep the sort by encoded key. No package-level cache: the order is decided per
call, as fxamacker caches per type.
