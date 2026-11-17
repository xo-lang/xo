kind: perf
module: gif
result: fail
commit: 502a0170c
source: 9ff8887d8d7d89358e9d71c93832b3267ae0f8abebf66d3c5bef42b461c8f78f
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" gif
measure: 3 cases of 1 twin directories, worst Xo / Go 1.251 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gif; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gif/encode | 25.8 | 32.2 | 1.251 |
| bench/perf/gif/decode | 29.1 | 28.7 | 0.989 |
| bench/perf/gif/none (under 5 ms: startup) | 2.0 | 2.0 | 0.985 |
