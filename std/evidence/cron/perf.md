kind: perf
module: cron
result: pass
commit: 117bb3891
source: ad70c0523204c0df68995a026e2b80a736d492145a8f12d8ebcce1def10e9993
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" cron
measure: 4 cases of 1 twin directories, worst Xo / Go 0.216 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/cron; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/cron/none (under 5 ms: startup) | 0.7 | 0.5 | 0.808 |
| bench/perf/cron/every5 | 99.5 | 17.5 | 0.176 |
| bench/perf/cron/weekday_ny | 245.4 | 53.0 | 0.216 |
| bench/perf/cron/monthly_last (under 5 ms: startup) | 2.7 | 2.2 | 0.827 |
