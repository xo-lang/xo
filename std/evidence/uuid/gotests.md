kind: gotests
module: uuid
result: pass
commit: 72766fc68
source: d3872a64dccaa15719cd8b16d26ff5dcc04361bb678fdf38b7e71f83e0113c7d
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" uuid
measure: 105 cases from 3 tables of Go's tests (go1.26.11, github.com/google/uuid v1.6.0), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/uuid/gotests/go) and the module (std/evidence/uuid/gotests/main.xo), every result equal, error text included.

- github.com/google/uuid uuid_test.go tests (parse): 48 cases, 0 skipped
- github.com/google/uuid uuid_test.go tests (parseup): 48 cases, 0 skipped
- github.com/google/uuid uuid_test.go TestValidate.testCases (parse): 9 cases, 0 skipped

- arm64: 105 cases, 0 mismatches, 4ms
- llvm: 105 cases, 0 mismatches, 5ms
