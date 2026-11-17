kind: perf
module: random
result: fail
commit: e918cf3b5
source: 472f5da38e7e959093060f9f3163e86a91c3fc3d44b8a80aef62c6d59810407b
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" random
measure: 5 cases of 1 twin directories, worst Xo / Go 1.839 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/random; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/random/none (under 5 ms: startup) | 0.6 | 0.5 | 0.820 |
| bench/perf/random/normal | 9.6 | 13.0 | 1.361 |
| bench/perf/random/exp | 6.8 | 10.7 | 1.567 |
| bench/perf/random/perm | 14.1 | 24.1 | 1.706 |
| bench/perf/random/zipf | 22.1 | 40.6 | 1.839 |
