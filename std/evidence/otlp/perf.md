kind: perf
module: otlp
result: pass
commit: 8ea9d8269
source: 1a17816c9de67a47b175a7963041699c25d69774732c31e696ae1b645ae8d727
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" otlp
measure: 3 cases of 1 twin directories, worst Xo / Go 0.463 over the cases above 5 ms (Proven at or under 1.05), 24s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/otlp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/otlp/none (under 5 ms: startup) | 3.1 | 1.3 | 0.425 |
| bench/perf/otlp/traces | 1165.6 | 257.9 | 0.221 |
| bench/perf/otlp/metrics | 434.1 | 201.1 | 0.463 |
