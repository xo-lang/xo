kind: perf
module: embed
result: pass
commit: 502a0170c
source: 5db1ac841a64a96945999b65a42306ad09ed4ca436cc238b35252db0a2741592
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" embed
measure: 4 cases of 1 twin directories, worst Xo / Go 0.594 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/embed; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/embed/none (under 5 ms: startup) | 0.6 | 0.5 | 0.877 |
| bench/perf/embed/read | 270.9 | 157.7 | 0.582 |
| bench/perf/embed/text | 264.4 | 156.9 | 0.594 |
| bench/perf/embed/sub | 62.1 | 31.0 | 0.499 |
