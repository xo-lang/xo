kind: gotests
module: compress
result: pass
commit: 7e7457fc4
source: f51ab22d3d6118fe4cecf839d9a848b5336c93e87c5a754aa40f25b5d9792ec5
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" compress
measure: 61 cases from 5 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 2 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/compress/gotests/go) and the module (std/evidence/compress/gotests/main.xo), every result equal, error text included.

- compress/zlib reader_test.go zlibTests (zlib): 14 cases, 0 skipped
- compress/gzip gunzip_test.go gunzipTests (gunzip): 14 cases, 1 skipped
- compress/flate inflate_test.go TestReaderTruncated.vectors (inflate): 10 cases, 0 skipped
- compress/flate deflate_test.go deflateTests (deflate): 14 cases, 1 skipped
- compress/lzw reader_test.go lzwTests (lzw): 9 cases, 0 skipped

- arm64: 61 cases, 0 mismatches, 7ms
- llvm: 61 cases, 0 mismatches, 6ms
