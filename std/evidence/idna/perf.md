kind: perf
module: idna
result: pass
commit: 5a23d3a84
source: 42f8c8ab1b0739a36a2e892fcd6d33c315ce2529171ec83ec0380997fe313450
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" idna
measure: 8 cases of 1 twin directories, worst Xo / Go 0.995 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/idna; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/idna/none (under 5 ms: startup) | 0.7 | 0.5 | 0.734 |
| bench/perf/idna/ascii | 26.7 | 15.7 | 0.588 |
| bench/perf/idna/mixed | 21.2 | 20.2 | 0.954 |
| bench/perf/idna/long | 5.5 | 4.3 | 0.793 |
| bench/perf/idna/map (under 5 ms: startup) | 4.2 | 3.1 | 0.745 |
| bench/perf/idna/nfcmap | 14.5 | 5.7 | 0.390 |
| bench/perf/idna/enc | 9.4 | 8.4 | 0.889 |
| bench/perf/idna/dec | 13.9 | 13.8 | 0.995 |
