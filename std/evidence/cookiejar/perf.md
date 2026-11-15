kind: perf
module: cookiejar
result: pass
commit: 117bb3891
source: 48dda45750f054426a395f76597b9da7bc84aad3970694a9b23f16e2bbab68eb
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" cookiejar
measure: 4 cases of 1 twin directories, worst Xo / Go 0.887 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/cookiejar; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/cookiejar/none (under 5 ms: startup) | 0.8 | 0.5 | 0.677 |
| bench/perf/cookiejar/set | 78.7 | 55.1 | 0.700 |
| bench/perf/cookiejar/get | 114.6 | 99.3 | 0.867 |
| bench/perf/cookiejar/both | 205.0 | 181.7 | 0.887 |
