kind: gotests
module: uniclass
result: pass
commit: 7e7885a5a
source: 87fb24873656d07b207bb28a302d371e8afbc3bba56ba417169d9968e843c66e
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" uniclass
measure: 74 cases from 2 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/uniclass/gotests/go) and the module (std/evidence/uniclass/gotests/main.xo), every result equal, error text included.

- unicode script_test.go inCategoryTest (class): 39 cases, 0 skipped
- unicode script_test.go inPropTest (class): 35 cases, 0 skipped

- arm64: 74 cases, 0 mismatches, 5ms
- llvm: 74 cases, 0 mismatches, 5ms
