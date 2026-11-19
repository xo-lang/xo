kind: gotests
module: multipart
result: pass
commit: 321e737a2
source: 88b5f62618177bf9e2c5faaec049db6c2fb7cc845d834242f9fd2467cedca7b6
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" multipart
measure: 17 cases from 2 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/multipart/gotests/go) and the module (std/evidence/multipart/gotests/main.xo), every result equal, error text included.

- mime/multipart writer_test.go TestWriterSetBoundary.tests (boundary): 11 cases, 0 skipped
- mime/multipart writer_test.go TestFileContentDisposition.tests (disp): 6 cases, 0 skipped

- arm64: 17 cases, 0 mismatches, 7ms
- llvm: 17 cases, 0 mismatches, 5ms
