kind: perf
module: protoparse
result: pass
commit: 536b01c2b
source: d94c20a22e602ed1fee180a50d59a6a3402b86ef66e36b549b66fd014dd27507
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protoparse
measure: 2 cases of 1 twin directories, worst Xo / Go 0.094 over the cases above 5 ms (Proven at or under 1.05), 9s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protoparse; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protoparse/none (under 5 ms: startup) | 1.0 | 0.7 | 0.677 |
| bench/perf/protoparse/parse | 702.4 | 66.1 | 0.094 |
