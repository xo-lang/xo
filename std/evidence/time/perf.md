kind: perf
module: time
result: fail
commit: 85d121b31
source: 52d151223a9301d7248388c786c862f04d9a0840ead501f389a70294c5132851
date: 2026-11-14
host: linux/arm64
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64" time
measure: 10 cases of 2 twin directories, worst Xo / Go 1.087 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/time, bench/perf/timego; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/time/none (under 5 ms: startup) | 0.7 | 0.5 | 0.800 |
| bench/perf/time/format | 35.4 | 38.4 | 1.087 |
| bench/perf/time/parse | 18.4 | 10.3 | 0.559 |
| bench/perf/time/civil | 22.2 | 8.4 | 0.381 |
| bench/perf/time/duration | 14.7 | 10.9 | 0.740 |
| bench/perf/time/format_layout | 35.1 | 29.2 | 0.830 |
| bench/perf/time/parse_layout | 17.5 | 9.3 | 0.532 |
| bench/perf/timego/none (under 5 ms: startup) | 0.5 | 0.5 | 0.850 |
| bench/perf/timego/format | 33.3 | 32.4 | 0.975 |
| bench/perf/timego/parse | 8.9 | 8.6 | 0.974 |
