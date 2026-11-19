kind: gotests
module: tabwriter
result: pass
commit: 11939d312
source: 3c8c7d20fed3259169c9750ba55aabe00a7dc7c4e8dd820355f9d4221827d6a0
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" tabwriter
measure: 45 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 8 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/tabwriter/gotests/go) and the module (std/evidence/tabwriter/gotests/main.xo), every result equal, error text included.

- text/tabwriter tabwriter_test.go tests (format): 45 cases, 8 skipped

- arm64: 45 cases, 0 mismatches, 5ms
- llvm: 45 cases, 0 mismatches, 6ms
