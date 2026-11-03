# std/cron changes

## 2026-11-03: feature: std/cron, crontab schedules and their next fire time

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 8: cron schedules); decision 0474 (Proposed)
- Go reference: github.com/robfig/cron/v3 v3.0.1 (parser.go, spec.go, constantdelay.go), MIT license
- Test: internal/stdtest/testdata/corpus/cron (robfig/cron's ParseStandard and Next on 400+ seeded specs in nine zones from ten start times each, DST gaps and overlaps, `@every`, 35 malformed specs) on arm64, LLVM and the Go backend; bench/perf/cron twincheck against robfig itself
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`cron.parse`, `Schedule.next`, `Schedule.zone` (spec/std.md 58).
