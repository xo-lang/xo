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
