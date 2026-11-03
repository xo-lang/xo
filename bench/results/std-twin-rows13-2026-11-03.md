# Timing rows against Go, part 13 (2026-11-03)

Linux/arm64 offload host, `xo bench --ab` style runs (15 rounds, Go and Xo alternated, caches off) of bench/perf/cron: `Schedule.next` (decision 0474) against robfig/cron v3.0.1's `Schedule.Next`, each fire time computed from the one before.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| cron | none | 0.651 | 0.34 | 0.521 | 0.474-0.599 | 0.697 |
| cron | every5 (`*/5 * * * *`, 200000 times) | 99.49 | 18.6 | 0.187 | 0.185-0.189 | 0.731 |
| cron | weekday_ny (`TZ=America/New_York 30 9 * * 1-5`, 20000 times) | 244.2 | 60.95 | 0.25 | 0.249-0.253 | 0.496 |
| cron | monthly_last (`0 0 31 1,3,5,7,8,10,12 *`, 1000 times) | 2.663 | 10.17 | 3.821 | 3.724-3.885 | 0.473 |

Two of three schedules are well under Go. Not Proven: `monthly_last` is 3.8 of Go's wall. It skips whole months and then days that cannot match, and each step here builds a civil time through the zone while robfig's does `AddDate` arithmetic; the run is also short (1000 times, 2.7 ms in Go), so fixed costs weigh more. Next: skip a non matching day by arithmetic in `next` instead of a civil conversion per day, then re-measure with more iterations.
