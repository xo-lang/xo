kind: gotests
module: asn1
result: pass
commit: 40b23cca6
source: 444cc881e206a4aee1a351db2d7ccb45ba615961ed8c03cb621cc8aaebdf117f
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" asn1
measure: 117 cases from 10 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/asn1/gotests/go) and the module (std/evidence/asn1/gotests/main.xo), every result equal, error text included.

- encoding/asn1 asn1_test.go boolTestData (bool): 5 cases, 0 skipped
- encoding/asn1 asn1_test.go int64TestData (int64): 12 cases, 0 skipped
- encoding/asn1 asn1_test.go int32TestData (int32): 12 cases, 0 skipped
- encoding/asn1 asn1_test.go bigIntTests (bigint): 9 cases, 0 skipped
- encoding/asn1 asn1_test.go bitStringTestData (bitstring): 6 cases, 0 skipped
- encoding/asn1 asn1_test.go bitStringRightAlignTests (rightalign): 6 cases, 0 skipped
- encoding/asn1 asn1_test.go objectIdentifierTestData (oid): 6 cases, 0 skipped
- encoding/asn1 asn1_test.go utcTestData (utc): 29 cases, 0 skipped
- encoding/asn1 asn1_test.go generalizedTimeTestData (gen): 24 cases, 0 skipped
- encoding/asn1 asn1_test.go bmpStringTests (bmp): 8 cases, 0 skipped

- arm64: 117 cases, 0 mismatches, 8ms
- llvm: 117 cases, 0 mismatches, 5ms
