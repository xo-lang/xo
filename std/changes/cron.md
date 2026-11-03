# std/cron changes

## 2026-11-03: feature: std/cron, crontab schedules and their next fire time

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 8: cron schedules); decision 0474 (Accepted, owner 2026-11-03)
- Go reference: github.com/robfig/cron/v3 v3.0.1 (parser.go, spec.go, constantdelay.go), MIT license
- Test: internal/stdtest/testdata/corpus/cron (robfig/cron's ParseStandard and Next on 400+ seeded specs in nine zones from ten start times each, DST gaps and overlaps, `@every`, 35 malformed specs) on arm64, LLVM and the Go backend; bench/perf/cron twincheck against robfig itself
- Timing: bench/results/std-twin-rows13-2026-11-03.md: every5 0.19, weekday_ny 0.25, monthly_last 3.82 of Go's wall (not Proven for that schedule; skipping non matching days by arithmetic is next)

`cron.parse`, `Schedule.next`, `Schedule.zone` (spec/std.md 58).
