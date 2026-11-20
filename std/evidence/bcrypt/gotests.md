kind: gotests
module: bcrypt
result: pass
commit: 78078e9b2
source: 65b06f6a1cb7021bcf1a4f0760a23078339b43d06364f11ac6258bca84bf51ee
date: 2026-11-20
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" bcrypt
measure: 5 cases from 1 table of Go's tests (go1.26.11, golang.org/x/crypto v0.59.0), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/bcrypt/gotests/go) and the module (std/evidence/bcrypt/gotests/main.xo), every result equal, error text included.

- golang.org/x/crypto/bcrypt bcrypt_test.go invalidTests (invalid): 5 cases, 0 skipped

- arm64: 5 cases, 0 mismatches, 20ms
- llvm: 5 cases, 0 mismatches, 10ms
