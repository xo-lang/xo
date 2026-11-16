kind: perf
module: protowkt
result: pass
commit: 536b01c2b
source: 71aa8667324c85879da6fb2cbbee81473a99978f131e226f47afd212b2fd4b0c
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protowkt
measure: 3 cases of 1 twin directories, worst Xo / Go 0.509 over the cases above 5 ms (Proven at or under 1.05), 11s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protowkt; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protowkt/none (under 5 ms: startup) | 0.9 | 0.5 | 0.560 |
| bench/perf/protowkt/timestamp | 122.0 | 30.4 | 0.249 |
| bench/perf/protowkt/struct | 559.2 | 284.7 | 0.509 |
