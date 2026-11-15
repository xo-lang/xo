kind: perf
module: metrics
result: fail
commit: 374c25851
source: 6d8032fa9efadc0dbc6edeb31e1263d6570e5a4e27083a197e40f357118225c4
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" metrics
measure: 6 cases of 1 twin directories, worst Xo / Go 1.134 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/metrics; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/metrics/none (under 5 ms: startup) | 1.2 | 0.5 | 0.433 |
| bench/perf/metrics/counter | 72.7 | 82.4 | 1.134 |
| bench/perf/metrics/histogram | 77.7 | 67.6 | 0.870 |
| bench/perf/metrics/counterheld (under 5 ms: startup) | 4.3 | 4.3 | 0.997 |
| bench/perf/metrics/histogramheld | 27.8 | 22.7 | 0.818 |
| bench/perf/metrics/scrape | 110.0 | 63.5 | 0.577 |
