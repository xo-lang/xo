kind: gotests
module: protobuf
result: pass
commit: ae728b448
source: 60304805bf37b3c5deea63383cba965beb1f1a614dd538624505c54449ececc3
date: 2026-11-20
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" protobuf
measure: 13 cases from 1 table of Go's tests (go1.26.11, google.golang.org/protobuf v1.36.12), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/protobuf/gotests/go) and the module (std/evidence/protobuf/gotests/main.xo), every result equal, error text included.

- google.golang.org/protobuf/encoding/protowire wire_test.go TestZigZag.tests (zigzag): 13 cases, 0 skipped

- arm64: 13 cases, 0 mismatches, 14ms
- llvm: 13 cases, 0 mismatches, 7ms
