kind: gotests
module: base64
result: pass
commit: 2d7a559a3
source: 72ae507dcf3440a70a26748637303a606d92b9a812febe8c53af61d12a521084
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" base64
measure: 57 cases from 3 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 3 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/base64/gotests/go) and the module (std/evidence/base64/gotests/main.xo), every result equal, error text included.

- encoding/base64 base64_test.go pairs (decode): 18 cases, 0 skipped
- encoding/base64 base64_test.go pairs (encode): 15 cases, 3 skipped
- encoding/base64 base64_test.go TestDecodeCorrupt.testCases (decode): 24 cases, 0 skipped

- arm64: 57 cases, 0 mismatches, 9ms
- llvm: 57 cases, 0 mismatches, 5ms
