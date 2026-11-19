kind: gotests
module: http
result: pass
commit: 237cafab1
source: d6f6572b686d1f417c0b931bbc300cbe5a6532b6593e170b5fc427c645264fbc
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" http
measure: 244 cases from 5 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 1 element skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/http/gotests/go) and the module (std/evidence/http/gotests/main.xo), every result equal, error text included.

- net/url url_test.go urltests (parse_url): 61 cases, 0 skipped
- net/url url_test.go TestParseErrors.tests (parse_url): 44 cases, 0 skipped
- net/url url_test.go TestParseStrictIpv6.tests (parse_url): 3 cases, 0 skipped
- net/url url_test.go parseRequestURLTests (parse_url): 43 cases, 1 skipped
- net/url url_test.go resolveReferenceTests (parse_url): 93 cases, 0 skipped

- arm64: 244 cases, 0 mismatches, 5ms
- llvm: 244 cases, 0 mismatches, 6ms
