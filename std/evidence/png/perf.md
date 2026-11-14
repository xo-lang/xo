kind: perf
module: png
result: pass
commit: 6781cd66a
source: 55deeec6c4c71321c3b5898464da5887ca6835c68a46c1a2e9f59a572b5d108d
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" png
measure: 3 cases of 1 twin directories, worst Xo / Go 1.024 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/png; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/png/encode | 342.1 | 319.6 | 0.934 |
| bench/perf/png/decode | 61.2 | 62.7 | 1.024 |
| bench/perf/png/none | 39.2 | 37.5 | 0.957 |
