kind: perf
module: websocket
result: fail
commit: 117bb3891
source: c8b61576b3c98a8b0216221b2096284e461ffba6b5ead05f712a057dc0bcb388
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" websocket
measure: 4 cases of 1 twin directories, worst Xo / Go 1.085 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/websocket; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/websocket/none (under 5 ms: startup) | 1.0 | 0.9 | 0.868 |
| bench/perf/websocket/small | 75.9 | 82.4 | 1.085 |
| bench/perf/websocket/big | 112.3 | 43.2 | 0.384 |
| bench/perf/websocket/dial | 47.3 | 33.1 | 0.700 |
