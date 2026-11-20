kind: perf
module: random
result: fail
commit: 1a40dc9d7
source: 15e5a4744726f53cd5b6ce3c16178cbb26dae015d854da9a77cd551d32d45cc0
date: 2026-11-20
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" random
measure: 6 cases of 1 twin directories, worst Xo / Go 1.221 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/random; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/random/none (under 5 ms: startup) | 1.4 | 1.1 | 0.809 |
| bench/perf/random/normal | 11.6 | 9.0 | 0.780 |
| bench/perf/random/exp | 8.0 | 6.7 | 0.845 |
| bench/perf/random/perm | 18.6 | 12.5 | 0.672 |
| bench/perf/random/zipf | 27.2 | 33.2 | 1.221 |
| bench/perf/random/draw | 211.1 | 143.0 | 0.677 |
