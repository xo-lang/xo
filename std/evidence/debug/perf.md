kind: perf
module: debug
result: fail
commit: 502a0170c
source: 6598532a2b83121f816b0ce71f00b3d19ae709874626f74070add7297a237a73
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" debug
measure: 4 cases of 1 twin directories, worst Xo / Go 1.125 over the cases above 5 ms (Proven at or under 1.05), 13s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/debug; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/debug/none (under 5 ms: startup) | 0.8 | 0.5 | 0.646 |
| bench/perf/debug/refused | 263.0 | 75.2 | 0.286 |
| bench/perf/debug/sched | 303.2 | 83.0 | 0.274 |
| bench/perf/debug/mem | 186.7 | 210.0 | 1.125 |
