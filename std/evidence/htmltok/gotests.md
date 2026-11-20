kind: gotests
module: htmltok
result: pass
commit: 96845a849
source: 3219a730d707097fcb1a191b2b21b160b3393c043a0e97d7c993d58488fa23a7
date: 2026-11-20
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" htmltok
measure: 248 cases from 3 tables of Go's tests (go1.26.11, golang.org/x/net v0.62.0), 2 backends (arm64,llvm), 0 mismatches with Go, 2 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/htmltok/gotests/go) and the module (std/evidence/htmltok/gotests/main.xo), every result equal, error text included.

- golang.org/x/net/html token_test.go tokenTests (tokens): 116 cases, 1 skipped
- golang.org/x/net/html token_test.go tokenTests (raw): 116 cases, 1 skipped
- golang.org/x/net/html escape_test.go unescapeTests (unescape): 16 cases, 0 skipped

- arm64: 248 cases, 0 mismatches, 7ms
- llvm: 248 cases, 0 mismatches, 5ms
