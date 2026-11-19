kind: gotests
module: bufio
result: pass
commit: d09e249af
source: b73395b0d06fa722309e46e884633a4d2b8dabae0194508599f1e7820af5a968
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" bufio
measure: 68 cases from 8 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/bufio/gotests/go) and the module (std/evidence/bufio/gotests/main.xo), every result equal, error text included.

- bufio scan_test.go scanTests (lines): 9 cases, 0 skipped
- bufio scan_test.go scanTests (lines1): 9 cases, 0 skipped
- bufio scan_test.go scanTests (until): 9 cases, 0 skipped
- bufio scan_test.go scanTests (until1): 9 cases, 0 skipped
- bufio scan_test.go wordScanTests (lines): 8 cases, 0 skipped
- bufio scan_test.go wordScanTests (lines1): 8 cases, 0 skipped
- bufio scan_test.go wordScanTests (until): 8 cases, 0 skipped
- bufio scan_test.go wordScanTests (until1): 8 cases, 0 skipped

- arm64: 68 cases, 0 mismatches, 5ms
- llvm: 68 cases, 0 mismatches, 5ms
