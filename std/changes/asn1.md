# std/asn1 changes

## 2026-11-11: port: std/asn1, Go's encoding/asn1 as a DER value tree (decision 0601 stage 1)

- Kind: port
- Commit: this push
- Found by: std audit 2026-11-11 (bench/results/std-audit-2026-11-11.md, rank 5: no public ASN.1 API)
- Go reference: encoding/asn1 (asn1.go, marshal.go of go1.26.1)
- Test: TestStdCorpora asn1 on go, arm64, llvm (1305 cases against Go's Unmarshal and Marshal); std/asn1_test.xo
- Shipped in: not yet released

A new module: parse, parse_prefix, Value.der, oid, bit_at, right_align
and tagged_as over a Value tree (spec/std.md 78) with Go's DER rules and
error texts. Not ported: Marshal and Unmarshal of structs by field tags
(derive Asn1 is decision 0601 stage 2).
