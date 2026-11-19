kind: gotests
module: time
result: pass
commit: 237cafab1
source: 3ee9d9a8783aa796429e09948f19ba204933601a20d1d78b2c3e97ffd1cc1093
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" time
measure: 19 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 2 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/time/gotests/go) and the module (std/evidence/time/gotests/main.xo), every result equal, error text included.

- time time_test.go dateTests (date): 19 cases, 2 skipped

- arm64: 19 cases, 0 mismatches, 5ms
- llvm: 19 cases, 0 mismatches, 5ms
