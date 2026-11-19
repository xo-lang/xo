kind: gotests
module: png
result: pass
commit: e8e191dcc
source: f037bc6cf9d4c493a73c10ba536419d3984a1b40f671e27b52fb88b95b7e144f
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" png
measure: 44 cases from 3 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/png/gotests/go) and the module (std/evidence/png/gotests/main.xo), every result equal, error text included.

- image/png reader_test.go filenames (decode): 35 cases, 0 skipped
- image/png reader_test.go filenamesPaletted (config): 5 cases, 0 skipped
- image/png reader_test.go readerErrors (decode): 4 cases, 0 skipped

- arm64: 44 cases, 0 mismatches, 6ms
- llvm: 44 cases, 0 mismatches, 7ms
