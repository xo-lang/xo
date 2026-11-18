kind: perf
module: websocket
result: pass
commit: 8b0b348a0
source: e8b70b2694fdfff0e727b692d621c86390c8e89da89dabbee02cd0dbd214296b
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" websocket
measure: 4 cases of 1 twin directories, worst Xo / Go 0.925 over the cases above 5 ms (Proven at or under 1.05), 18s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/websocket; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/websocket/none (under 5 ms: startup) | 3.2 | 2.2 | 0.696 |
| bench/perf/websocket/small | 280.8 | 259.7 | 0.925 |
| bench/perf/websocket/big | 359.6 | 127.2 | 0.354 |
| bench/perf/websocket/dial | 186.8 | 169.5 | 0.907 |
