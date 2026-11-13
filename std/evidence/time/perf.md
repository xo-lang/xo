kind: perf
module: time
result: fail
commit: 5fabb1593
source: 0f6e03a54e9e61ee09562f14dd53ab46392f441e93698ac906abdc440e7652ec
date: 2026-11-13
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" time
measure: 10 cases of 2 twin directories, worst Xo / Go 1.540 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/time, bench/perf/timego; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/time/none (under 5 ms: startup) | 0.7 | 0.5 | 0.720 |
| bench/perf/time/format | 35.2 | 38.4 | 1.089 |
| bench/perf/time/parse | 18.3 | 16.9 | 0.927 |
| bench/perf/time/civil | 22.0 | 8.0 | 0.364 |
| bench/perf/time/duration | 14.7 | 10.8 | 0.733 |
| bench/perf/time/format_layout | 34.8 | 28.2 | 0.811 |
| bench/perf/time/parse_layout | 18.1 | 15.3 | 0.846 |
| bench/perf/timego/none (under 5 ms: startup) | 0.7 | 0.5 | 0.693 |
| bench/perf/timego/format | 33.3 | 44.3 | 1.328 |
| bench/perf/timego/parse | 9.1 | 14.0 | 1.540 |
