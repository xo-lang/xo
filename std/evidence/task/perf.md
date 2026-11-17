kind: perf
module: task
result: fail
commit: 502a0170c
source: da5632d9b15808daa9c8579c4869b7fa9b083f628e34aefb4d33984f28c0530d
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" task
measure: 4 cases of 1 twin directories, worst Xo / Go 18.680 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/task; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/task/none (under 5 ms: startup) | 0.6 | 0.5 | 0.887 |
| bench/perf/task/map | 75.4 | 98.8 | 1.311 |
| bench/perf/task/once | 2.1 | 39.1 | 18.680 |
| bench/perf/task/sem | 41.9 | 105.0 | 2.506 |
