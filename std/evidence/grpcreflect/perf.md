kind: perf
module: grpcreflect
result: pass
commit: aba8ca5b1
source: 9b15086fde88ef4adf9ce680d36b978e6e71ef8bd4a250ce974a06ed8426d208
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" grpcreflect
measure: 3 cases of 1 twin directories, worst Xo / Go 0.531 over the cases above 5 ms (Proven at or under 1.05), 21s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/grpcreflect; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/grpcreflect/none (under 5 ms: startup) | 3.6 | 1.5 | 0.415 |
| bench/perf/grpcreflect/stream | 939.6 | 499.3 | 0.531 |
| bench/perf/grpcreflect/files | 200.5 | 52.0 | 0.259 |
