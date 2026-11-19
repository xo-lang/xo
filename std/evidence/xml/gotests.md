kind: gotests
module: xml
result: pass
commit: d1b9d1c63
source: eabed39f6a91c9b36cec510fac8c0c63dd45c67e8fefa9bed6b80a8a2f872714
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" xml
measure: 150 cases from 16 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 20 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/xml/gotests/go) and the module (std/evidence/xml/gotests/main.xo), every result equal, error text included.

- encoding/xml xml_test.go TestIssue12417.testCases (tok): 4 cases, 0 skipped
- encoding/xml xml_test.go TestIssue20396.testCases (tok): 8 cases, 0 skipped
- encoding/xml xml_test.go TestIssue20685.testCases (tok): 7 cases, 0 skipped
- encoding/xml xml_test.go TestParseErrors.tests (tok): 1 cases, 10 skipped
- encoding/xml xml_test.go TestIssue11405.testCases (tok): 3 cases, 0 skipped
- encoding/xml read_test.go TestMalformedComment.testData (tok): 4 cases, 0 skipped
- encoding/xml xml_test.go characterTests (tok): 9 cases, 0 skipped
- encoding/xml xml_test.go xmlInput (tok): 39 cases, 0 skipped
- encoding/xml xml_test.go TestIssue12417.testCases (raw): 4 cases, 0 skipped
- encoding/xml xml_test.go TestIssue20396.testCases (raw): 8 cases, 0 skipped
- encoding/xml xml_test.go TestIssue20685.testCases (raw): 7 cases, 0 skipped
- encoding/xml xml_test.go TestParseErrors.tests (raw): 1 cases, 10 skipped
- encoding/xml xml_test.go TestIssue11405.testCases (raw): 3 cases, 0 skipped
- encoding/xml read_test.go TestMalformedComment.testData (raw): 4 cases, 0 skipped
- encoding/xml xml_test.go characterTests (raw): 9 cases, 0 skipped
- encoding/xml xml_test.go xmlInput (raw): 39 cases, 0 skipped

- arm64: 150 cases, 0 mismatches, 29ms
- llvm: 150 cases, 0 mismatches, 5ms
