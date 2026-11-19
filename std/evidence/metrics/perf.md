kind: perf
module: metrics
result: fail
commit: 541463aee
source: 7748864704ce4ef209a7b6610390809cc55c2b1e82998eb48ca814964ec281a2
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" metrics
measure: 6 cases of 1 twin directories, worst Xo / Go 1.077 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/metrics; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/metrics/none (under 5 ms: startup) | 3.4 | 1.2 | 0.365 |
| bench/perf/metrics/counter | 90.5 | 87.7 | 0.968 |
| bench/perf/metrics/histogram | 96.6 | 104.0 | 1.077 |
| bench/perf/metrics/counterheld | 17.1 | 13.9 | 0.813 |
| bench/perf/metrics/histogramheld | 45.7 | 35.1 | 0.769 |
| bench/perf/metrics/scrape | 167.3 | 96.5 | 0.577 |
