kind: gotests
module: publicsuffix
result: pass
commit: 518f3d020
source: 04450658eed33d05e18b444831a7f52fd0fe19129103f2e0a0f1278a8b81b873
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" publicsuffix
measure: 139 cases from 2 tables of Go's tests (go1.26.11, golang.org/x/net v0.62.0), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/publicsuffix/gotests/go) and the module (std/evidence/publicsuffix/gotests/main.xo), every result equal, error text included.

- golang.org/x/net/publicsuffix list_test.go publicSuffixTestCases (ps): 71 cases, 0 skipped
- golang.org/x/net/publicsuffix list_test.go eTLDPlusOneTestCases (etld1): 68 cases, 0 skipped

- arm64: 139 cases, 0 mismatches, 3ms
- llvm: 139 cases, 0 mismatches, 4ms
