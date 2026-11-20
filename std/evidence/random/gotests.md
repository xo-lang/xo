kind: gotests
module: random
result: pass
commit: 661a41f8b
source: 15e5a4744726f53cd5b6ce3c16178cbb26dae015d854da9a77cd551d32d45cc0
date: 2026-11-20
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" random
measure: 372 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/random/gotests/go) and the module (std/evidence/random/gotests/main.xo), every result equal, error text included.

- math/rand/v2 chacha8_test.go chacha8output (chacha8): 372 cases, 0 skipped

- arm64: 372 cases, 0 mismatches, 16ms
- llvm: 372 cases, 0 mismatches, 15ms
