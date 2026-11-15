kind: perf
module: regexp
result: fail
commit: 117bb3891
source: 13803fe02c249e60c74af423ae176dae2cc96252e1bbd1e9dcd20c46c056212c
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" regexp
measure: 6 cases of 1 twin directories, worst Xo / Go 1.100 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/regexp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/regexp/none (under 5 ms: startup) | 1.0 | 0.8 | 0.801 |
| bench/perf/regexp/compile | 73.3 | 58.7 | 0.801 |
| bench/perf/regexp/match | 50.7 | 55.7 | 1.100 |
| bench/perf/regexp/find_all | 98.4 | 80.7 | 0.819 |
| bench/perf/regexp/replace_all | 70.3 | 70.2 | 0.998 |
| bench/perf/regexp/captures | 9.8 | 8.6 | 0.876 |
