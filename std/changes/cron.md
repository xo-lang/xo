# std/cron changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus cron on go, arm64 and llvm

std/cron.xo now holds the bodies of std/native/cron.xo (removed), and the Go
backend compiles them from it as its own package, in place of the hand written std/rt/cron.go (removed): the Go
backend now runs the same cron code as the native backends. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-03: perf: a schedule skips days that cannot match without a civil conversion

- Kind: perf
- Commit: this commit
- Found by: the std timing rows (std-twin-rows13: monthly_last 3.82 of robfig/cron)
- Go reference: robfig/cron v3.0.1 Schedule.Next (AddDate(0, 0, 1) per day)
- Test: internal/stdtest corpus cron (439 zone cases) on the native and Go backends; bench/perf/cron twincheck
- Timing: monthly_last 0.857 of robfig's wall (was 3.821), every5 0.194, weekday_ny 0.243 on linux/arm64 (bench/results/std-twin-rows17-2026-11-03.md)
- Shipped in: not yet released

`next` steps a day that cannot match by arithmetic when the zone's offset
is the same at the next day's clock, instead of building the civil time
through the zone each day; fire times are unchanged.

## 2026-11-03: feature: std/cron, crontab schedules and their next fire time

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 8: cron schedules); decision 0474 (Accepted, owner 2026-11-03)
- Go reference: github.com/robfig/cron/v3 v3.0.1 (parser.go, spec.go, constantdelay.go), MIT license
- Test: internal/stdtest/testdata/corpus/cron (robfig/cron's ParseStandard and Next on 400+ seeded specs in nine zones from ten start times each, DST gaps and overlaps, `@every`, 35 malformed specs) on arm64, LLVM and the Go backend; bench/perf/cron twincheck against robfig itself
- Timing: bench/results/std-twin-rows13-2026-11-03.md: every5 0.19, weekday_ny 0.25, monthly_last 3.82 of Go's wall (not Proven for that schedule; skipping non matching days by arithmetic is next)

`cron.parse`, `Schedule.next`, `Schedule.zone` (spec/std.md 58).
