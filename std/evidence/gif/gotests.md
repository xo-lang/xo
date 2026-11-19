kind: gotests
module: gif
result: pass
commit: 321e737a2
source: 145345a868dfdca64726b17042f5b8664aecbbda17e4dd5926004ce4288bbaa8
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" gif
measure: 3 cases from 1 table of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/gif/gotests/go) and the module (std/evidence/gif/gotests/main.xo), every result equal, error text included.

- image/gif reader_test.go TestLoopCount.testCases (loop): 3 cases, 0 skipped

- arm64: 3 cases, 0 mismatches, 5ms
- llvm: 3 cases, 0 mismatches, 4ms
