kind: perf
module: random
result: fail
commit: ce4d4fdcd
source: 472f5da38e7e959093060f9f3163e86a91c3fc3d44b8a80aef62c6d59810407b
date: 2026-11-19
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" random
measure: 5 cases of 1 twin directories, worst Xo / Go 1.591 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/random; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/random/none (under 5 ms: startup) | 0.5 | 0.4 | 0.887 |
| bench/perf/random/normal | 8.9 | 12.1 | 1.366 |
| bench/perf/random/exp | 6.6 | 9.7 | 1.454 |
| bench/perf/random/perm | 14.1 | 9.5 | 0.672 |
| bench/perf/random/zipf | 19.6 | 31.2 | 1.591 |
