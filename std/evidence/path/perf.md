kind: perf
module: path
result: fail
commit: 09f097a2d
source: 032ac42177123a49d723e2980db00521eb1f1898fef9374f5091a60ab5ec8a56
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" path
measure: 4 cases of 1 twin directories, worst Xo / Go 1.076 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/path; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/path/clean | 9.0 | 5.9 | 0.653 |
| bench/perf/path/join | 22.1 | 14.7 | 0.667 |
| bench/perf/path/parts | 7.5 | 6.7 | 0.894 |
| bench/perf/path/match | 6.1 | 6.5 | 1.076 |
