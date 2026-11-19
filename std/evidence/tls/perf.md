kind: perf
module: tls
result: pass
commit: 5659a7193
source: 188aa613c46253365806eb774b7303f7b39d61f9bee2170e77fd7353bed25704
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" tls
measure: 2 cases of 1 twin directories, worst Xo / Go 0.761 over the cases above 5 ms (Proven at or under 1.05), 4m5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/tlsloop; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/tlsloop/small | 5392.5 | 4101.7 | 0.761 |
| bench/perf/tlsloop/big | 9332.0 | 5552.7 | 0.595 |
