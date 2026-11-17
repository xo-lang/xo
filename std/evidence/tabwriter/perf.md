kind: perf
module: tabwriter
result: fail
commit: 502a0170c
source: 7728e32c10ff5333dd094d0661115573632ebb1f79b678aea89e248f16978b19
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" tabwriter
measure: 3 cases of 1 twin directories, worst Xo / Go 1.084 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/tabwriter; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/tabwriter/none (under 5 ms: startup) | 0.6 | 0.5 | 0.823 |
| bench/perf/tabwriter/pad | 31.0 | 33.2 | 1.070 |
| bench/perf/tabwriter/right | 30.9 | 33.5 | 1.084 |
