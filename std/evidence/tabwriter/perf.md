kind: perf
module: tabwriter
result: pass
commit: 669f74f7a
source: 3c8c7d20fed3259169c9750ba55aabe00a7dc7c4e8dd820355f9d4221827d6a0
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" tabwriter
measure: 3 cases of 1 twin directories, worst Xo / Go 0.873 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/tabwriter; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/tabwriter/none (under 5 ms: startup) | 0.6 | 0.5 | 0.796 |
| bench/perf/tabwriter/pad | 31.7 | 27.4 | 0.865 |
| bench/perf/tabwriter/right | 31.3 | 27.3 | 0.873 |
