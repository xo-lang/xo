kind: perf
module: task
result: pass
commit: c977db0ad
source: 664d9550755dc831ae3863b24b570e3940ef7ac410605b2a45d403139446765c
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" task
measure: 4 cases of 1 twin directories, worst Xo / Go 0.568 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/task; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/task/none (under 5 ms: startup) | 1.4 | 1.1 | 0.787 |
| bench/perf/task/map | 82.5 | 41.9 | 0.508 |
| bench/perf/task/once (under 5 ms: startup) | 3.0 | 2.4 | 0.817 |
| bench/perf/task/sem | 56.2 | 31.9 | 0.568 |
