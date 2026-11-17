kind: perf
module: image
result: fail
commit: 57d0505e3
source: 7c927933e66698de2e6b0ba3efbbb523cef4a1b0fb11209429cc1b51bd881c0c
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" image
measure: 5 cases of 1 twin directories, worst Xo / Go 1.437 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/image; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/image/over | 17.2 | 24.7 | 1.437 |
| bench/perf/image/src | 19.2 | 25.4 | 1.321 |
| bench/perf/image/gray | 13.2 | 15.2 | 1.152 |
| bench/perf/image/none | 9.7 | 9.9 | 1.018 |
| bench/perf/image/copy | 10.0 | 13.4 | 1.338 |
