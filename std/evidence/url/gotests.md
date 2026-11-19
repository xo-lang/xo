kind: gotests
module: url
result: pass
commit: 237cafab1
source: c38d7ac81fd915afd9682ebeb0f1aa98b80d112ddfe1ecd7ae0b5c89b3511bb7
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" url
measure: 307 cases from 10 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 1 element skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/url/gotests/go) and the module (std/evidence/url/gotests/main.xo), every result equal, error text included.

- net/url url_test.go urltests (parse): 61 cases, 0 skipped
- net/url url_test.go TestParseErrors.tests (parse): 44 cases, 0 skipped
- net/url url_test.go TestParseStrictIpv6.tests (parse): 3 cases, 0 skipped
- net/url url_test.go parseRequestURLTests (request): 43 cases, 1 skipped
- net/url url_test.go unescapeTests (unescape): 13 cases, 0 skipped
- net/url url_test.go queryEscapeTests (query_escape): 5 cases, 0 skipped
- net/url url_test.go pathEscapeTests (path_escape): 7 cases, 0 skipped
- net/url url_test.go resolveReferenceTests (resolve): 93 cases, 0 skipped
- net/url url_test.go parseTests (parse_query): 17 cases, 0 skipped
- net/url url_test.go TestURLHostnameAndPort.tests (hostport): 21 cases, 0 skipped

- arm64: 307 cases, 0 mismatches, 7ms
- llvm: 307 cases, 0 mismatches, 8ms
