kind: perf
module: time
result: fail
commit: da2ba49a9
source: 3684fae2a5b3c72d15a343a0497bdf5423603ad9ff6c57f80a4b857e699350b7
date: 2026-11-14
host: linux/arm64
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64" time
measure: 10 cases of 2 twin directories, worst Xo / Go 1.237 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/time, bench/perf/timego; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/time/none (under 5 ms: startup) | 0.7 | 0.5 | 0.757 |
| bench/perf/time/format | 35.6 | 39.7 | 1.117 |
| bench/perf/time/parse | 18.1 | 12.4 | 0.688 |
| bench/perf/time/civil | 21.8 | 8.3 | 0.380 |
| bench/perf/time/duration | 14.7 | 10.8 | 0.733 |
| bench/perf/time/format_layout | 35.1 | 29.0 | 0.828 |
| bench/perf/time/parse_layout | 18.1 | 10.3 | 0.565 |
| bench/perf/timego/none (under 5 ms: startup) | 0.6 | 0.4 | 0.652 |
| bench/perf/timego/format | 33.5 | 41.4 | 1.237 |
| bench/perf/timego/parse | 9.0 | 10.8 | 1.210 |
