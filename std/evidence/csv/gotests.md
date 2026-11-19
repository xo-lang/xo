kind: gotests
module: csv
result: pass
commit: 4e635fa80
source: 0d21951aad72463ccd1ca57f473f066f703965afdb07f93cec594f84428f3806
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" csv
measure: 64 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 4 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/csv/gotests/go) and the module (std/evidence/csv/gotests/main.xo), every result equal, error text included.

- encoding/csv reader_test.go readTests (read): 64 cases, 4 skipped

- arm64: 64 cases, 0 mismatches, 4ms
- llvm: 64 cases, 0 mismatches, 4ms
