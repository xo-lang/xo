kind: gotests
module: cron
result: pass
commit: 72766fc68
source: ed800243e98ac48391c08e54a0557267d56bf575d5fe3dc901d063e9b6324fb1
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" cron
measure: 96 cases from 4 tables of Go's tests (go1.26.11, github.com/robfig/cron/v3 v3.0.1), 2 backends (arm64,llvm), 0 mismatches with Go, 0 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/cron/gotests/go) and the module (std/evidence/cron/gotests/main.xo), every result equal, error text included.

- github.com/robfig/cron/v3 spec_test.go TestActivation.tests (act): 29 cases, 0 skipped
- github.com/robfig/cron/v3 spec_test.go TestNext.runs (next): 58 cases, 0 skipped
- github.com/robfig/cron/v3 spec_test.go TestNextWithTz.runs (nextstd): 4 cases, 0 skipped
- github.com/robfig/cron/v3 parser_test.go TestParseScheduleErrors.tests (perr): 5 cases, 0 skipped

- arm64: 96 cases, 0 mismatches, 4ms
- llvm: 96 cases, 0 mismatches, 4ms
