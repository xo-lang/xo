kind: perf
module: image
result: fail
commit: 502a0170c
source: 74ed74126c6cc618b91e9537a8cd0eaa32c5ab7fcac21338011288103213c269
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" image
measure: 5 cases of 1 twin directories, worst Xo / Go 1.401 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/image; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/image/over | 16.9 | 23.7 | 1.401 |
| bench/perf/image/src | 18.9 | 24.2 | 1.280 |
| bench/perf/image/gray | 12.8 | 14.1 | 1.102 |
| bench/perf/image/none | 9.3 | 9.1 | 0.981 |
| bench/perf/image/copy | 9.7 | 12.4 | 1.282 |
