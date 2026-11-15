kind: perf
module: path
result: fail
commit: eaa07eb3b
source: 765a2daaba2ad94d932fef403b5aea58bbbf321978457fc7e5776b1b42751368
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" path
measure: 4 cases of 1 twin directories, worst Xo / Go 1.110 over the cases above 5 ms (Proven at or under 1.05), 1s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/path; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/path/clean | 6.2 | 5.9 | 0.941 |
| bench/perf/path/join | 17.0 | 18.3 | 1.082 |
| bench/perf/path/parts | 6.4 | 7.1 | 1.110 |
| bench/perf/path/match | 5.9 | 5.8 | 0.984 |
