kind: perf
module: metrics
result: fail
commit: 09f097a2d
source: 81dbc8c1e9c6feb785d49b3428c300f0133c481c4893d834e7c3d177af04c253
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" metrics
measure: 6 cases of 1 twin directories, worst Xo / Go 1.664 over the cases above 5 ms (Proven at or under 1.05), 13s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/metrics; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/metrics/none (under 5 ms: startup) | 3.5 | 1.2 | 0.334 |
| bench/perf/metrics/counter | 92.1 | 153.3 | 1.664 |
| bench/perf/metrics/histogram | 98.8 | 136.1 | 1.377 |
| bench/perf/metrics/counterheld | 16.9 | 13.8 | 0.815 |
| bench/perf/metrics/histogramheld | 45.7 | 35.6 | 0.780 |
| bench/perf/metrics/scrape | 171.4 | 104.3 | 0.609 |
