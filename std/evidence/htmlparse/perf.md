kind: perf
module: htmlparse
result: fail
commit: 7645bf7aa
source: 1ca093deeaba1b8afd8310c403274d3456bd21a9a0806b04b41404e06790f0ef
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" htmlparse
measure: 5 cases of 1 twin directories, worst Xo / Go 1.136 over the cases above 5 ms (Proven at or under 1.05), 16s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/htmlparse; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/htmlparse/none (under 5 ms: startup) | 0.7 | 0.5 | 0.725 |
| bench/perf/htmlparse/parse | 224.9 | 188.8 | 0.840 |
| bench/perf/htmlparse/tok | 140.6 | 87.9 | 0.625 |
| bench/perf/htmlparse/ent | 368.9 | 419.1 | 1.136 |
| bench/perf/htmlparse/render | 51.5 | 47.8 | 0.928 |
