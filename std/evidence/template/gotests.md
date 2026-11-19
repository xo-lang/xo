kind: gotests
module: template
result: pass
commit: cd84499d3
source: cdbafec31a1043f126ad25db1df3764137f5ebfa1f0d5d38ed361065e5a8900e
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" template
measure: 600 cases from 4 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 13 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/template/gotests/go) and the module (std/evidence/template/gotests/main.xo), every result equal, error text included.

- text/template exec_test.go execTests (exec): 343 cases, 13 skipped
- text/template exec_test.go cmpTests (cmp): 117 cases, 0 skipped
- text/template/parse parse_test.go parseTests (parse): 108 cases, 0 skipped
- text/template/parse parse_test.go errorTests (parse): 32 cases, 0 skipped

- arm64: 600 cases, 0 mismatches, 24ms
- llvm: 600 cases, 0 mismatches, 9ms
