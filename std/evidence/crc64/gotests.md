kind: gotests
module: crc64
result: pass
commit: 11939d312
source: 3bd2152457fa866fea42d7ce226f5e0ed10c3d399dff43376e90ef16b1c1f199
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" crc64
measure: 32 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/crc64/gotests/go) and the module (std/evidence/crc64/gotests/main.xo), every result equal, error text included.

- hash/crc64 crc64_test.go golden (sum): 32 cases, 0 skipped

- arm64: 32 cases, 0 mismatches, 5ms
- llvm: 32 cases, 0 mismatches, 5ms
