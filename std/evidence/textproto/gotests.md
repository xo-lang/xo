kind: gotests
module: textproto
result: pass
commit: 15fe9056e
source: 3ee6c206038dafad864801f91bf39b968b19e5e2a161726cd9d33bc0552b0da8
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" textproto
measure: 18 cases from 2 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/textproto/gotests/go) and the module (std/evidence/textproto/gotests/main.xo), every result equal, error text included.

- net/textproto header_test.go canonicalHeaderKeyTests (canonical): 14 cases, 0 skipped
- net/textproto reader_test.go readResponseTests (response): 4 cases, 0 skipped

- arm64: 18 cases, 0 mismatches, 8ms
- llvm: 18 cases, 0 mismatches, 4ms
