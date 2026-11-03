# Timing rows against Go, part 17 (2026-11-03)

Linux/arm64 offload host (the first, exclusive). bench/perf/cron at
95b06977e against robfig/cron v3.0.1 (Go 1.26.1): `xo bench --ab` with Go
as A and Xo as B, 15 rounds (twin-cron.txt; script twin.sh).

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| cron | none | 0.617 | 0.533 | 0.864 | 0.808-0.915 | 0.211 |
| cron | every5 | 103.7 | 20.15 | 0.194 | 0.177-0.264 | 0.21 |
| cron | weekday_ny | 249.4 | 60.63 | 0.243 | 0.238-0.283 | 0.351 |
| cron | monthly_last | 2.774 | 2.378 | 0.857 | 0.846-0.89 | 0.211 |

`monthly_last` was 3.821 of Go's wall (std-twin-rows13-2026-11-03.md).
`Schedule.next` converted a civil time through the zone for every day that
could not match, about 30 a fire. It now steps such a day by arithmetic:
the next day at the same clock, read as UTC, is one day on, and when the
zone's offset there is the one Go's time.Date lookup returns, the result
and its civil time need no conversion. The parent against the change on
the same host (ab-host, 3 rounds, ab-cron.txt): wall 10.4 to 2.423 ms,
B/A 0.233 (0.232-0.237). All three schedules are Proven better than Go.
