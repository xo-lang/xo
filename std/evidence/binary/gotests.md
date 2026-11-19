kind: gotests
module: binary
result: pass
commit: fac765340
source: 02147b7cb939d66e0697a2161aa78b96c96d22ade23a0fb28e81e1aa120ded1a
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" binary
measure: 21 cases from 2 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 1 element skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/binary/gotests/go) and the module (std/evidence/binary/gotests/main.xo), every result equal, error text included.

- encoding/binary varint_test.go tests (varint): 18 cases, 0 skipped
- encoding/binary varint_test.go TestBufferTooBigWithOverflow.tests (uvarint): 3 cases, 1 skipped

- arm64: 21 cases, 0 mismatches, 7ms
- llvm: 21 cases, 0 mismatches, 5ms
