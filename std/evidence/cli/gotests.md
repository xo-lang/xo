kind: gotests
module: cli
result: pass
commit: 7e7885a5a
source: 06436b6976603b015b6652c833d7421c05016815a644e9e402eabcc9cad840d0
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" cli
measure: 5 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/cli/gotests/go) and the module (std/evidence/cli/gotests/main.xo), every result equal, error text included.

- flag flag_test.go TestExitCode.tests (exit): 5 cases, 0 skipped

- arm64: 5 cases, 0 mismatches, 4ms
- llvm: 5 cases, 0 mismatches, 5ms
