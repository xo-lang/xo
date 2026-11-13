kind: perf
module: json
result: pass
commit: 92eff6e90
source: 57a1aa4d4abc6193c4bc0845d6619bd8f3fee2e3c5b0a9911ee742d760a72820
date: 2026-11-13
host: linux/arm64
command: go run ./scripts/stdproven perf -rounds 7 -host "linux/arm64" json
measure: 25 cases of 8 twin directories, worst Xo / Go 0.980 over the cases above 5 ms (Proven at or under 1.05), 2m15s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 7 over bench/perf/jsonf32, bench/perf/jsonindent, bench/perf/jsonmap, bench/perf/jsonown, bench/perf/jsontag, bench/perf/jsontoken, bench/perf/jsontwin, bench/perf/jsonvalue; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/jsonf32/none (under 5 ms: startup) | 0.9 | 0.5 | 0.622 |
| bench/perf/jsonf32/encode | 34.6 | 20.0 | 0.579 |
| bench/perf/jsonindent/none (under 5 ms: startup) | 0.8 | 0.5 | 0.602 |
| bench/perf/jsonindent/indent | 125.6 | 85.2 | 0.679 |
| bench/perf/jsonindent/rawfield | 163.0 | 105.8 | 0.649 |
| bench/perf/jsonmap/small | 15.3 | 8.5 | 0.552 |
| bench/perf/jsonmap/large | 56.9 | 37.5 | 0.659 |
| bench/perf/jsonmap/none (under 5 ms: startup) | 0.6 | 0.4 | 0.671 |
| bench/perf/jsonown/none | 15.7 | 3.2 | 0.204 |
| bench/perf/jsonown/strfield | 729.6 | 477.8 | 0.655 |
| bench/perf/jsonown/via | 1186.5 | 299.8 | 0.253 |
| bench/perf/jsonown/value | 974.6 | 309.2 | 0.317 |
| bench/perf/jsontag/none (under 5 ms: startup) | 0.9 | 0.6 | 0.697 |
| bench/perf/jsontag/external | 1792.6 | 166.6 | 0.093 |
| bench/perf/jsontag/tagfirst | 1668.8 | 151.6 | 0.091 |
| bench/perf/jsontag/taglast | 2305.6 | 182.2 | 0.079 |
| bench/perf/jsontag/onepass | 1251.0 | 175.8 | 0.141 |
| bench/perf/jsontoken/none (under 5 ms: startup) | 1.0 | 0.6 | 0.620 |
| bench/perf/jsontoken/read | 1071.2 | 285.2 | 0.266 |
| bench/perf/jsontoken/write | 76.0 | 74.5 | 0.980 |
| bench/perf/jsontwin/decode | 108.9 | 26.7 | 0.245 |
| bench/perf/jsontwin/encode | 33.2 | 16.1 | 0.485 |
| bench/perf/jsonvalue/none (under 5 ms: startup) | 1.6 | 0.7 | 0.472 |
| bench/perf/jsonvalue/to | 682.2 | 28.2 | 0.041 |
| bench/perf/jsonvalue/from | 682.0 | 108.6 | 0.159 |
