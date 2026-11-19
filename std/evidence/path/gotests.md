kind: gotests
module: path
result: pass
commit: 7c3ff6898
source: 06588d4ee5d27b5325d46b0881493e7c5278ba0733ca2488cbcbe944f952f0bb
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" path
measure: 170 cases from 8 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/path/gotests/go) and the module (std/evidence/path/gotests/main.xo), every result equal, error text included.

- path path_test.go cleantests (clean): 36 cases, 0 skipped
- path path_test.go cleantests (clean): 36 cases, 0 skipped
- path path_test.go splittests (split): 5 cases, 0 skipped
- path path_test.go exttests (ext): 5 cases, 0 skipped
- path path_test.go basetests (base): 11 cases, 0 skipped
- path path_test.go dirtests (dir): 13 cases, 0 skipped
- path path_test.go isAbsTests (abs): 8 cases, 0 skipped
- path match_test.go matchTests (match): 56 cases, 0 skipped

- arm64: 170 cases, 0 mismatches, 5ms
- llvm: 170 cases, 0 mismatches, 3ms
