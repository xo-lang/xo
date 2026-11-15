kind: perf
module: rate
result: pass
commit: 117bb3891
source: cd853e8b90d7d1c60b16e46442ac94f8e5badbb03dd1ddf3946a97a307ad2313
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" rate
measure: 2 cases of 1 twin directories, worst Xo / Go 0.569 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/rate; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/rate/none (under 5 ms: startup) | 3.1 | 2.8 | 0.916 |
| bench/perf/rate/allow | 325.9 | 185.4 | 0.569 |
