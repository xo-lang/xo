kind: perf
module: metrics
result: fail
commit: b232ddf22
source: b2fc7259e88ce07bd03b070fd485b03192ce7fa10a12fe3298a532f5bed09566
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" metrics
measure: 6 cases of 1 twin directories, worst Xo / Go 1.265 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/metrics; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/metrics/none (under 5 ms: startup) | 1.1 | 0.5 | 0.460 |
| bench/perf/metrics/counter | 73.7 | 93.3 | 1.265 |
| bench/perf/metrics/histogram | 76.1 | 72.0 | 0.945 |
| bench/perf/metrics/counterheld (under 5 ms: startup) | 4.2 | 3.9 | 0.945 |
| bench/perf/metrics/histogramheld | 27.7 | 22.1 | 0.797 |
| bench/perf/metrics/scrape | 108.6 | 92.2 | 0.849 |
