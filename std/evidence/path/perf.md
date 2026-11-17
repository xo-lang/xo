kind: perf
module: path
result: fail
commit: 57d0505e3
source: 032ac42177123a49d723e2980db00521eb1f1898fef9374f5091a60ab5ec8a56
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" path
measure: 4 cases of 1 twin directories, worst Xo / Go 1.135 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/path; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/path/clean | 6.3 | 5.5 | 0.870 |
| bench/perf/path/join | 17.2 | 19.5 | 1.135 |
| bench/perf/path/parts | 6.1 | 6.5 | 1.057 |
| bench/perf/path/match | 5.4 | 6.0 | 1.096 |
