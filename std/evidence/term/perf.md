kind: perf
module: term
result: pass
commit: 251eaa332
source: 76e2f95e4d3f53f5c173988d84d60da19236595861d8e514b59e9960271da7c9
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" term
measure: 7 cases of 2 twin directories, worst Xo / Go 1.033 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/term, bench/perf/termline; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/term/none (under 5 ms: startup) | 0.6 | 0.5 | 0.783 |
| bench/perf/term/tty | 42.2 | 35.5 | 0.841 |
| bench/perf/term/paint | 20.2 | 19.4 | 0.962 |
| bench/perf/term/strip | 13.6 | 13.2 | 0.970 |
| bench/perf/termline/none (under 5 ms: startup) | 1.4 | 1.0 | 0.753 |
| bench/perf/termline/type | 32.1 | 33.1 | 1.031 |
| bench/perf/termline/wrap | 32.9 | 34.0 | 1.033 |
