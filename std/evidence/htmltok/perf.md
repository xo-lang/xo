kind: perf
module: htmltok
result: pass
commit: 7645bf7aa
source: 21a458616820c0381de591ccff92def28e736a681fd1c4d380653a64b531c013
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" htmltok
measure: 3 cases of 1 twin directories, worst Xo / Go 0.714 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/htmltok; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/htmltok/none (under 5 ms: startup) | 0.7 | 0.5 | 0.739 |
| bench/perf/htmltok/next | 101.3 | 72.3 | 0.714 |
| bench/perf/htmltok/token | 218.0 | 142.0 | 0.651 |
