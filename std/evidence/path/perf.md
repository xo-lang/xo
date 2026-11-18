kind: perf
module: path
result: fail
commit: b232ddf22
source: 032ac42177123a49d723e2980db00521eb1f1898fef9374f5091a60ab5ec8a56
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" path
measure: 4 cases of 1 twin directories, worst Xo / Go 1.129 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/path; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/path/clean | 6.2 | 6.1 | 0.979 |
| bench/perf/path/join | 17.2 | 19.4 | 1.129 |
| bench/perf/path/parts | 6.8 | 7.0 | 1.026 |
| bench/perf/path/match | 5.5 | 5.6 | 1.027 |
