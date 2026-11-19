kind: gotests
module: cookiejar
result: pass
commit: 074861457
source: 0091b86941589f1a53c263521a86d1c2d1d7652be3c72e91d2f999433d3fb2e1
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" cookiejar
measure: 73 cases from 6 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 3 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/cookiejar/gotests/go) and the module (std/evidence/cookiejar/gotests/main.xo), every result equal, error text included.

- net/http/cookiejar jar_test.go basicsTests (jar): 20 cases, 0 skipped
- net/http/cookiejar jar_test.go updateAndDeleteTests (jar): 11 cases, 1 skipped
- net/http/cookiejar jar_test.go chromiumBasicsTests (jar): 18 cases, 0 skipped
- net/http/cookiejar jar_test.go chromiumDomainTests (jar): 7 cases, 0 skipped
- net/http/cookiejar jar_test.go chromiumDeletionTests (jar): 6 cases, 2 skipped
- net/http/cookiejar jar_test.go domainHandlingTests (jar): 11 cases, 0 skipped

- arm64: 73 cases, 0 mismatches, 9ms
- llvm: 73 cases, 0 mismatches, 4ms
