kind: perf
module: image
result: fail
commit: 8ea9d8269
source: 8ffded16ae4fadee5d85c93fcb7c29c5ae23528d24cc44b4c72a232733e8d927
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" image
measure: 5 cases of 1 twin directories, worst Xo / Go 1.129 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/image; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/image/over | 26.2 | 29.5 | 1.129 |
| bench/perf/image/src | 26.8 | 28.2 | 1.055 |
| bench/perf/image/gray | 19.2 | 15.2 | 0.790 |
| bench/perf/image/none | 14.3 | 10.2 | 0.713 |
| bench/perf/image/copy | 14.6 | 13.1 | 0.899 |
