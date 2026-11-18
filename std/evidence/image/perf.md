kind: perf
module: image
result: fail
commit: 8b0b348a0
source: 7c927933e66698de2e6b0ba3efbbb523cef4a1b0fb11209429cc1b51bd881c0c
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" image
measure: 5 cases of 1 twin directories, worst Xo / Go 1.060 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/image; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/image/over | 25.9 | 27.5 | 1.060 |
| bench/perf/image/src | 26.9 | 26.0 | 0.968 |
| bench/perf/image/gray | 19.9 | 15.0 | 0.754 |
| bench/perf/image/none | 14.3 | 10.3 | 0.719 |
| bench/perf/image/copy | 14.8 | 13.1 | 0.882 |
