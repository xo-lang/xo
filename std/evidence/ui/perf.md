kind: perf
module: ui
result: fail
commit: 502a0170c
source: 3a30da528de933bd154a7c994d296766664f39c272759e5140226d9cc34a2093
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" ui
measure: 3 cases of 1 twin directories, worst Xo / Go 1.124 over the cases above 5 ms (Proven at or under 1.05), 11s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/ui; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/ui/none (under 5 ms: startup) | 0.7 | 0.6 | 0.834 |
| bench/perf/ui/render | 388.9 | 437.1 | 1.124 |
| bench/perf/ui/build | 78.5 | 30.3 | 0.387 |
