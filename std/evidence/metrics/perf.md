kind: perf
module: metrics
result: fail
commit: b1bd0e348
source: 59137ccfce72f885c5c9d0bf28dd038080572354a877087eebf371a72abec77b
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" metrics
measure: 6 cases of 1 twin directories, worst Xo / Go 1.640 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/metrics; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/metrics/none (under 5 ms: startup) | 1.3 | 0.6 | 0.443 |
| bench/perf/metrics/counter | 73.1 | 119.9 | 1.640 |
| bench/perf/metrics/histogram | 77.6 | 87.4 | 1.127 |
| bench/perf/metrics/counterheld (under 5 ms: startup) | 4.3 | 4.4 | 1.030 |
| bench/perf/metrics/histogramheld | 27.7 | 23.2 | 0.837 |
| bench/perf/metrics/scrape | 108.6 | 52.0 | 0.479 |
