kind: gotests
module: adler32
result: pass
commit: 11939d312
source: 3404e8e15f502d2d00e20369198cc5818488606ad73071f781b925f39c579f9e
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" adler32
measure: 32 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 11 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/adler32/gotests/go) and the module (std/evidence/adler32/gotests/main.xo), every result equal, error text included.

- hash/adler32 adler32_test.go golden (sum): 32 cases, 11 skipped

- arm64: 32 cases, 0 mismatches, 5ms
- llvm: 32 cases, 0 mismatches, 5ms
