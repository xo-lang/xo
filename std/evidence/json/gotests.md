kind: gotests
module: json
result: pass
commit: a59446166
source: 64dbfcc84b907a1411a9741aed97a0a4047107bbf3c6a1b2be3cc203121241c5
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" json
measure: 123 cases from 9 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/json/gotests/go) and the module (std/evidence/json/gotests/main.xo), every result equal, error text included.

- encoding/json scanner_test.go TestValid.tests (valid): 6 cases, 0 skipped
- encoding/json number_test.go TestNumberIsValid.validTests (number): 46 cases, 0 skipped
- encoding/json number_test.go TestNumberIsValid.invalidTests (number): 21 cases, 0 skipped
- encoding/json decode_test.go TestUnmarshalSyntax.tests (syntax): 8 cases, 0 skipped
- encoding/json scanner_test.go TestIndentErrors.tests (syntax): 2 cases, 0 skipped
- encoding/json scanner_test.go TestCompactAndIndent.tests (compact): 10 cases, 0 skipped
- encoding/json scanner_test.go TestCompactAndIndent.tests (compact): 10 cases, 0 skipped
- encoding/json scanner_test.go TestCompactAndIndent.tests (indent): 10 cases, 0 skipped
- encoding/json scanner_test.go TestCompactAndIndent.tests (indent): 10 cases, 0 skipped

- arm64: 123 cases, 0 mismatches, 8ms
- llvm: 123 cases, 0 mismatches, 5ms
