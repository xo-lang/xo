kind: perf
module: random
result: pass
commit: de18b6b6e
source: 15e5a4744726f53cd5b6ce3c16178cbb26dae015d854da9a77cd551d32d45cc0
date: 2026-11-20
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" random
measure: 6 cases of 1 twin directories, worst Xo / Go 0.913 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/random; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go | Go package math ms (decision 0721) |
|---|---|---|---|---|
| bench/perf/random/none (under 5 ms: startup) | 1.4 | 1.2 | 0.808 | |
| bench/perf/random/normal | 11.7 | 9.5 | 0.811 | |
| bench/perf/random/exp | 8.2 | 6.6 | 0.802 | |
| bench/perf/random/perm | 18.5 | 12.7 | 0.688 | |
| bench/perf/random/zipf | 36.4 | 33.3 | 0.913 | 26.9 |
| bench/perf/random/draw | 201.6 | 143.4 | 0.711 | |
