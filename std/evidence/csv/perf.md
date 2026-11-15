kind: perf
module: csv
result: pass
commit: 191613523
source: 728c1ed0ac1a3f1926661234020f83f631735f01f06c88547d140fb6b4eb5027
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" csv
measure: 4 cases of 1 twin directories, worst Xo / Go 0.882 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/csv; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/csv/none | 5.1 | 2.5 | 0.486 |
| bench/perf/csv/parse | 49.1 | 30.9 | 0.628 |
| bench/perf/csv/format | 45.7 | 40.3 | 0.882 |
| bench/perf/csv/reader | 41.9 | 33.5 | 0.801 |
