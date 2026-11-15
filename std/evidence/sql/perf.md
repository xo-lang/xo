kind: perf
module: sql
result: pass
commit: 117bb3891
source: de4612187f5560c92771029a41cfd395c01f4541e7bffa0d1c83c07a1f030448
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" sql
measure: 2 cases of 1 twin directories, worst Xo / Go 0.820 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/sqlrow; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/sqlrow/none (under 5 ms: startup) | 0.6 | 1.8 | 2.843 |
| bench/perf/sqlrow/scan | 62.5 | 51.3 | 0.820 |
