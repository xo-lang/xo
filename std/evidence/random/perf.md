kind: perf
module: random
result: fail
commit: 040538186
source: 15e5a4744726f53cd5b6ce3c16178cbb26dae015d854da9a77cd551d32d45cc0
date: 2026-11-20
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" random
measure: 5 cases of 1 twin directories, worst Xo / Go 1.728 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/random; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/random/none (under 5 ms: startup) | 1.4 | 1.1 | 0.784 |
| bench/perf/random/normal | 11.7 | 16.2 | 1.386 |
| bench/perf/random/exp | 8.1 | 14.1 | 1.728 |
| bench/perf/random/perm | 18.6 | 15.8 | 0.850 |
| bench/perf/random/zipf | 27.0 | 40.6 | 1.506 |
