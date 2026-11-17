kind: perf
module: sse
result: fail
commit: 502a0170c
source: 292c125fe1271dd8946be889dd0be0d034800e7f88586f533d3ba448c2883789
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" sse
measure: 3 cases of 1 twin directories, worst Xo / Go 1.094 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/sse; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/sse/none (under 5 ms: startup) | 0.8 | 0.5 | 0.665 |
| bench/perf/sse/encode | 19.1 | 17.2 | 0.899 |
| bench/perf/sse/parse | 128.8 | 140.9 | 1.094 |
