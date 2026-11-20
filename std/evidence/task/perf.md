kind: perf
module: task
result: pass
commit: 58d699cd8
source: 66157ab96597d7fb265d16dba42286d2d6eb20cd13791096a8021f74614d8ea7
date: 2026-11-20
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" task
measure: 4 cases of 1 twin directories, worst Xo / Go 0.557 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/task; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/task/none (under 5 ms: startup) | 1.5 | 1.2 | 0.795 |
| bench/perf/task/map | 84.5 | 42.1 | 0.498 |
| bench/perf/task/once (under 5 ms: startup) | 2.8 | 2.4 | 0.866 |
| bench/perf/task/sem | 56.7 | 31.6 | 0.557 |
