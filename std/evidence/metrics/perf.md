kind: perf
module: metrics
result: fail
commit: ce0a8ffb1
source: eea56c760d78741a02351c5a834fc6edcaed5b87f8ea1f03ea1a752da6b542f5
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" metrics
measure: 6 cases of 1 twin directories, worst Xo / Go 1.339 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/metrics; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/metrics/none (under 5 ms: startup) | 1.2 | 0.5 | 0.449 |
| bench/perf/metrics/counter | 72.3 | 96.8 | 1.339 |
| bench/perf/metrics/histogram | 78.5 | 73.6 | 0.938 |
| bench/perf/metrics/counterheld (under 5 ms: startup) | 4.3 | 4.0 | 0.932 |
| bench/perf/metrics/histogramheld | 27.7 | 22.3 | 0.803 |
| bench/perf/metrics/scrape | 109.5 | 68.8 | 0.629 |
