kind: gotests
module: archive
result: pass
commit: b704d9fa3
source: 3f24b6af9b4c9eea91413ada780bcfa4a867ba1d1d386735136164f2a8b83a84
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" archive
measure: 65 cases from 2 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 3 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/archive/gotests/go) and the module (std/evidence/archive/gotests/main.xo), every result equal, error text included.

- archive/tar reader_test.go TestReader.vectors (tar): 33 cases, 0 skipped
- archive/zip reader_test.go tests (zip): 32 cases, 3 skipped

- arm64: 65 cases, 0 mismatches, 45ms
- llvm: 65 cases, 0 mismatches, 6ms
