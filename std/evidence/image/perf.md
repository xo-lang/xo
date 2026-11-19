kind: perf
module: image
result: fail
commit: 99d5a8117
source: c0f15de7edc4c229d609211de0f432e068b304ab70e65a790165de12f0a60df7
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" image
measure: 5 cases of 1 twin directories, worst Xo / Go 1.064 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/image; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/image/over | 26.2 | 27.9 | 1.064 |
| bench/perf/image/src | 26.5 | 25.1 | 0.948 |
| bench/perf/image/gray | 19.5 | 15.1 | 0.773 |
| bench/perf/image/none | 14.4 | 10.2 | 0.710 |
| bench/perf/image/copy | 14.6 | 13.4 | 0.917 |
