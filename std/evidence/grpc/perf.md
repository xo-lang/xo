kind: perf
module: grpc
result: pass
commit: 8b0b348a0
source: 9d94b6fcfc29d7555f764a7ab45bd8a90e5a8fa88d9cc77e1bfa9f65cc59609e
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" grpc
measure: 3 cases of 1 twin directories, worst Xo / Go 0.932 over the cases above 5 ms (Proven at or under 1.05), 1m28s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/grpc; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/grpc/none | 6.1 | 2.6 | 0.427 |
| bench/perf/grpc/unary | 4303.5 | 4009.0 | 0.932 |
| bench/perf/grpc/stream | 66.1 | 60.3 | 0.912 |
