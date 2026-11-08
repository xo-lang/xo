# std/cbor changes

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
