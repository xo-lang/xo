kind: gotests
module: crc32
result: pass
commit: 11939d312
source: 809902ba08de1d70147f64037f064aceb990ef7b235f0bfecb8c7afa96a3c39c
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" crc32
measure: 31 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 2 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/crc32/gotests/go) and the module (std/evidence/crc32/gotests/main.xo), every result equal, error text included.

- hash/crc32 crc32_test.go golden (sum): 31 cases, 2 skipped

- arm64: 31 cases, 0 mismatches, 5ms
- llvm: 31 cases, 0 mismatches, 6ms
