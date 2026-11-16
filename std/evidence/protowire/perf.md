kind: perf
module: protowire
result: pass
commit: 536b01c2b
source: 670224587f56b070dc0672c9146f624f5f839a7d433d7157ed294f2f25857c75
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protowire
measure: 3 cases of 1 twin directories, worst Xo / Go 0.763 over the cases above 5 ms (Proven at or under 1.05), 17s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protowire; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protowire/none (under 5 ms: startup) | 1.0 | 0.8 | 0.847 |
| bench/perf/protowire/write | 97.7 | 74.6 | 0.763 |
| bench/perf/protowire/walk | 1244.1 | 191.1 | 0.154 |
