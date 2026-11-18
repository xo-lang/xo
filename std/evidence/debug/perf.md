kind: perf
module: debug
result: fail
commit: b232ddf22
source: cfd491abb65e2618cee4777e67b2cc8a3d37f6e5b3c913ef604f755c3396f9f8
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" debug
measure: 4 cases of 1 twin directories, worst Xo / Go 1.112 over the cases above 5 ms (Proven at or under 1.05), 13s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/debug; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/debug/none (under 5 ms: startup) | 0.8 | 0.5 | 0.634 |
| bench/perf/debug/refused | 262.9 | 75.1 | 0.286 |
| bench/perf/debug/sched | 315.6 | 88.8 | 0.281 |
| bench/perf/debug/mem | 196.2 | 218.1 | 1.112 |
