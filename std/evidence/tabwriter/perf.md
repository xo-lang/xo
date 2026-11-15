kind: perf
module: tabwriter
result: fail
commit: 7645bf7aa
source: 44ad6b226d37198c57257a4d014b34a4373f0494002a0c27ecdf10f1158f3cee
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" tabwriter
measure: 3 cases of 1 twin directories, worst Xo / Go 1.162 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/tabwriter; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/tabwriter/none (under 5 ms: startup) | 0.6 | 0.5 | 0.794 |
| bench/perf/tabwriter/pad | 31.6 | 36.8 | 1.162 |
| bench/perf/tabwriter/right | 32.9 | 35.8 | 1.087 |
