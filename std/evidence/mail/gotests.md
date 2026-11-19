kind: gotests
module: mail
result: pass
commit: d2655fa2b
source: b6c890071e19f5e73701b5c8351657b662500723061d1da13477d43c7a2d7654
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" mail
measure: 131 cases from 6 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/mail/gotests/go) and the module (std/evidence/mail/gotests/main.xo), every result equal, error text included.

- net/mail message_test.go TestDateParsing.tests (date): 6 cases, 0 skipped
- net/mail message_test.go TestDateParsingCFWS.tests (date): 26 cases, 0 skipped
- net/mail message_test.go TestAddressParsing.tests (list): 40 cases, 0 skipped
- net/mail message_test.go TestAddressParser.tests (list): 14 cases, 0 skipped
- net/mail message_test.go TestAddressParsingAndFormatting.tests (addr): 27 cases, 0 skipped
- net/mail message_test.go TestAddressParsingAndFormatting.badTests (addr): 18 cases, 0 skipped

- arm64: 131 cases, 0 mismatches, 8ms
- llvm: 131 cases, 0 mismatches, 4ms
