kind: perf
module: heap
result: pass
commit: 117bb3891
source: 6634b34bc740fc63ab4dc0d88fb178420402f996360a614b22bf12fef0d3bc48
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" heap
measure: 4 cases of 1 twin directories, worst Xo / Go 0.942 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/heap; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/heap/none (under 5 ms: startup) | 0.6 | 0.5 | 0.820 |
| bench/perf/heap/sort | 87.5 | 82.4 | 0.942 |
| bench/perf/heap/steady | 159.1 | 135.7 | 0.853 |
| bench/perf/heap/fix | 33.5 | 30.7 | 0.916 |
