kind: gotests
module: timego
result: pass
commit: 3c8258c81
source: 158c57e5be523f0663f37185bf9f35a07866e76f06ec8ab2a4e21aa176e61a93
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" timego
measure: 192 cases from 8 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/timego/gotests/go) and the module (std/evidence/timego/gotests/main.xo), every result equal, error text included.

- time format_test.go formatTests (format): 29 cases, 0 skipped
- time format_test.go parseTests (parse): 68 cases, 0 skipped
- time format_test.go rubyTests (parse): 4 cases, 0 skipped
- time format_test.go parseErrorTests (parse): 36 cases, 0 skipped
- time format_test.go secondsTimeZoneOffsetTests (parse): 9 cases, 0 skipped
- time format_test.go dayOutOfRangeTests (ansic): 27 cases, 0 skipped
- time format_test.go monthOutOfRangeTests (month): 3 cases, 0 skipped
- time format_test.go longFractionalDigitsTests (rfc3339): 16 cases, 0 skipped

- arm64: 192 cases, 0 mismatches, 9ms
- llvm: 192 cases, 0 mismatches, 5ms
