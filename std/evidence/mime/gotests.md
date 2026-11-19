kind: gotests
module: mime
result: pass
commit: 321e737a2
source: e4396d2b892bc812e2ffa8e9c76bb4c3c022d9b6ef2966245daf5ab22319bddd
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" mime
measure: 54 cases from 3 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/mime/gotests/go) and the module (std/evidence/mime/gotests/main.xo), every result equal, error text included.

- mime encodedword_test.go TestDecodeWord.tests (word): 13 cases, 0 skipped
- mime encodedword_test.go TestDecodeHeader.tests (header): 28 cases, 0 skipped
- mime mediatype_test.go badMediaTypeTests (media): 13 cases, 0 skipped

- arm64: 54 cases, 0 mismatches, 8ms
- llvm: 54 cases, 0 mismatches, 5ms
