kind: gotests
module: charset
result: pass
commit: 0e939fab1
source: 0406c05bb9ad615c3799e31356da05ce77f23d292f7a1e605836330c588ed86c
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" charset
measure: 14 cases from 4 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/charset/gotests/go) and the module (std/evidence/charset/gotests/main.xo), every result equal, error text included.

- unicode/utf16 utf16_test.go encodeTests (encode): 3 cases, 0 skipped
- unicode/utf16 utf16_test.go encodeTests (decode): 3 cases, 0 skipped
- unicode/utf16 utf16_test.go decodeTests (decode): 4 cases, 0 skipped
- unicode/utf16 utf16_test.go decodeTests (encode): 4 cases, 0 skipped

- arm64: 14 cases, 0 mismatches, 9ms
- llvm: 14 cases, 0 mismatches, 5ms
