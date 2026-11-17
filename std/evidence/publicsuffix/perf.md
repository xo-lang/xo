kind: perf
module: publicsuffix
result: pass
commit: 502a0170c
source: 60f84c267d37cc3683dc2b69842a91cdfbccd64df5ed157ecea64b5b86cda2b9
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" publicsuffix
measure: 3 cases of 1 twin directories, worst Xo / Go 0.770 over the cases above 5 ms (Proven at or under 1.05), 14s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/publicsuffix; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/publicsuffix/none (under 5 ms: startup) | 0.8 | 0.6 | 0.744 |
| bench/perf/publicsuffix/suffix | 319.6 | 237.0 | 0.742 |
| bench/perf/publicsuffix/etld1 | 448.2 | 345.2 | 0.770 |
