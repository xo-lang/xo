kind: perf
module: metrics
result: pass
commit: 553a8b9bd
source: 92af3c3181fb1755d7300b1e811bdd63c4a504de6909251bd377c1b8cb025cbd
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" metrics
measure: 6 cases of 1 twin directories, worst Xo / Go 0.984 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/metrics; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/metrics/none (under 5 ms: startup) | 3.1 | 1.2 | 0.402 |
| bench/perf/metrics/counter | 89.1 | 87.7 | 0.984 |
| bench/perf/metrics/histogram | 96.7 | 88.5 | 0.916 |
| bench/perf/metrics/counterheld | 17.0 | 13.9 | 0.818 |
| bench/perf/metrics/histogramheld | 45.9 | 35.1 | 0.764 |
| bench/perf/metrics/scrape | 168.7 | 95.1 | 0.564 |
