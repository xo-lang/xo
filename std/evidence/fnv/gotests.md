kind: gotests
module: fnv
result: pass
commit: 2d7a559a3
source: 5f34cd0ac2005b37cf77ebf324b2f8d539b5827efe52c87aebbb39fc17218cba
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" fnv
measure: 24 cases from 6 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/fnv/gotests/go) and the module (std/evidence/fnv/gotests/main.xo), every result equal, error text included.

- hash/fnv fnv_test.go golden32 (32): 4 cases, 0 skipped
- hash/fnv fnv_test.go golden32a (32a): 4 cases, 0 skipped
- hash/fnv fnv_test.go golden64 (64): 4 cases, 0 skipped
- hash/fnv fnv_test.go golden64a (64a): 4 cases, 0 skipped
- hash/fnv fnv_test.go golden128 (128): 4 cases, 0 skipped
- hash/fnv fnv_test.go golden128a (128a): 4 cases, 0 skipped

- arm64: 24 cases, 0 mismatches, 5ms
- llvm: 24 cases, 0 mismatches, 4ms
