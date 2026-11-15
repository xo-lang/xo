kind: perf
module: idna
result: pass
commit: 1d09c6220
source: 43b4baadf7fdc861b507d754c17e8964dbf7cf90ebef483c2d75810202155e30
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" idna
measure: 8 cases of 1 twin directories, worst Xo / Go 1.047 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/idna; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/idna/none (under 5 ms: startup) | 0.7 | 0.5 | 0.734 |
| bench/perf/idna/ascii | 27.0 | 17.2 | 0.636 |
| bench/perf/idna/mixed | 21.6 | 20.9 | 0.967 |
| bench/perf/idna/long | 5.6 | 4.9 | 0.870 |
| bench/perf/idna/map (under 5 ms: startup) | 4.5 | 3.5 | 0.785 |
| bench/perf/idna/nfcmap | 15.1 | 6.1 | 0.407 |
| bench/perf/idna/enc | 9.7 | 9.3 | 0.959 |
| bench/perf/idna/dec | 14.0 | 14.7 | 1.047 |
