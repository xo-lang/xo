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
