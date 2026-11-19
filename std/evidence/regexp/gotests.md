kind: gotests
module: regexp
result: pass
commit: 8a6f53b45
source: 12eabe941e6fbce14ee9a1123aaf8ce6d95d74dec6655ca42588cb26227e4255
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" regexp
measure: 203 cases from 6 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 6 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/regexp/gotests/go) and the module (std/evidence/regexp/gotests/main.xo), every result equal, error text included.

- regexp all_test.go goodRe (compile): 17 cases, 0 skipped
- regexp all_test.go badRe (compile): 12 cases, 0 skipped
- regexp find_test.go findTests (find): 81 cases, 6 skipped
- regexp all_test.go replaceTests (replace): 72 cases, 0 skipped
- regexp all_test.go replaceLiteralTests (replacelit): 15 cases, 0 skipped
- regexp all_test.go metaTests (quote): 6 cases, 0 skipped

- arm64: 203 cases, 0 mismatches, 154ms
- llvm: 203 cases, 0 mismatches, 168ms
