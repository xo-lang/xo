kind: gotests
module: htmlparse
result: pass
commit: 70732382e
source: 8f55c5f822a75088c4ef956f01db8cbcfaee46c7e2ff5548d91b6fdf3a69932c
date: 2026-11-20
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" htmlparse
measure: 251 cases from 4 tables of Go's tests (go1.26.11, golang.org/x/net v0.62.0), 2 backends (arm64,llvm), 0 mismatches with Go, 2 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/htmlparse/gotests/go) and the module (std/evidence/htmlparse/gotests/main.xo), every result equal, error text included.

- golang.org/x/net/html token_test.go tokenTests (render): 116 cases, 1 skipped
- golang.org/x/net/html token_test.go tokenTests (noscript): 116 cases, 1 skipped
- golang.org/x/net/html escape_test.go unescapeTests (render): 16 cases, 0 skipped
- golang.org/x/net/html parse_test.go TestParseFragmentForeignContentTemplates.srcs (render): 3 cases, 0 skipped

- arm64: 251 cases, 0 mismatches, 9ms
- llvm: 251 cases, 0 mismatches, 5ms
