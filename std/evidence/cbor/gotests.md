kind: gotests
module: cbor
result: pass
commit: 4d25d7149
source: 59bfabec56f9fa40aa9385ee21df68881a46a0e9b2091dbabac56f7f75715516
date: 2026-11-20
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" cbor
measure: 289 cases from 8 tables of Go's tests (go1.26.11, github.com/fxamacker/cbor/v2 v2.9.0), 2 backends (arm64,llvm), 0 mismatches with Go, 1 element skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/cbor/gotests/go) and the module (std/evidence/cbor/gotests/main.xo), every result equal, error text included.

- github.com/fxamacker/cbor/v2 diagnose_test.go TestDiagnosticNotationExamples.testCases (diag): 81 cases, 0 skipped
- github.com/fxamacker/cbor/v2 diagnose_test.go TestDiagnoseByteString.testCases (diag): 18 cases, 0 skipped
- github.com/fxamacker/cbor/v2 diagnose_test.go TestDiagnoseTextString.testCases (diag): 12 cases, 0 skipped
- github.com/fxamacker/cbor/v2 diagnose_test.go TestDiagnoseInvalidTextString.testCases (diag): 3 cases, 0 skipped
- github.com/fxamacker/cbor/v2 diagnose_test.go TestDiagnoseFloatingPointNumber.testCases (diag): 7 cases, 0 skipped
- github.com/fxamacker/cbor/v2 diagnose_test.go TestDiagnoseTag.testCases (diag): 6 cases, 0 skipped
- github.com/fxamacker/cbor/v2 decode_test.go unmarshalTests (decode): 65 cases, 0 skipped
- github.com/fxamacker/cbor/v2 decode_test.go invalidCBORUnmarshalTests (decode): 97 cases, 1 skipped

- arm64: 289 cases, 0 mismatches, 8ms
- llvm: 289 cases, 0 mismatches, 4ms
