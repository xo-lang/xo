kind: perf
module: path
result: pass
commit: ad5a7dedb
source: 06588d4ee5d27b5325d46b0881493e7c5278ba0733ca2488cbcbe944f952f0bb
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" path
measure: 4 cases of 1 twin directories, worst Xo / Go 0.970 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/path; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/path/clean | 9.0 | 5.9 | 0.656 |
| bench/perf/path/join | 22.3 | 20.4 | 0.913 |
| bench/perf/path/parts | 7.6 | 6.5 | 0.847 |
| bench/perf/path/match | 6.1 | 5.9 | 0.970 |
