kind: perf
module: sqlite
result: pass
commit: f6c6cefb2
source: 9994afbcd8434dc12b04292b27b2275acb34663a668965dd9bcc17a12b9b22d3
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" sqlite
measure: 4 cases of 1 twin directories, worst Xo / Go 0.869 over the cases above 5 ms (Proven at or under 1.05), 14s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/sqlite; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/sqlite/none (under 5 ms: startup) | 0.8 | 0.9 | 1.140 |
| bench/perf/sqlite/insert | 32.9 | 28.6 | 0.869 |
| bench/perf/sqlite/point | 80.8 | 64.7 | 0.801 |
| bench/perf/sqlite/scan | 41.7 | 29.0 | 0.694 |
