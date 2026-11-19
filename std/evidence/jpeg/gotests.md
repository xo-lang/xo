kind: gotests
module: jpeg
result: pass
commit: e8e191dcc
source: bd4bf20338c33af9f5b44ab4506d000e6a30de93e57dd9b05e7f01305551f661
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" jpeg
measure: 26 cases from 3 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/jpeg/gotests/go) and the module (std/evidence/jpeg/gotests/main.xo), every result equal, error text included.

- image/jpeg reader_test.go TestDecodeProgressive.testCases (decode): 10 cases, 0 skipped
- image/jpeg reader_test.go TestDecodeProgressive.testCases (decode): 10 cases, 0 skipped
- image/jpeg writer_test.go testCase (encode): 6 cases, 0 skipped

- arm64: 26 cases, 0 mismatches, 15ms
- llvm: 26 cases, 0 mismatches, 15ms
