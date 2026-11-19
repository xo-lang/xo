kind: gotests
module: html
result: pass
commit: 11939d312
source: d4314c101efee4a15ef794ac181e839005540fbce3f12b56aff9c23b95bd022c
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" html
measure: 11 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/html/gotests/go) and the module (std/evidence/html/gotests/main.xo), every result equal, error text included.

- html escape_test.go unescapeTests (unescape): 11 cases, 0 skipped

- arm64: 11 cases, 0 mismatches, 5ms
- llvm: 11 cases, 0 mismatches, 6ms
