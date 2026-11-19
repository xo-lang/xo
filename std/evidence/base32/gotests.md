kind: gotests
module: base32
result: pass
commit: 2d7a559a3
source: 23946d7512d81ec981608c117ce478d8c69391a59ae69499ca20ad75178cf347
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" base32
measure: 53 cases from 3 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/base32/gotests/go) and the module (std/evidence/base32/gotests/main.xo), every result equal, error text included.

- encoding/base32 base32_test.go pairs (decode): 15 cases, 0 skipped
- encoding/base32 base32_test.go pairs (encode): 15 cases, 0 skipped
- encoding/base32 base32_test.go TestDecodeCorrupt.testCases (decode): 23 cases, 0 skipped

- arm64: 53 cases, 0 mismatches, 3ms
- llvm: 53 cases, 0 mismatches, 4ms
