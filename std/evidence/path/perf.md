kind: perf
module: path
result: fail
commit: 502a0170c
source: d98d36531b93880e3374d5e8b779068e1df0effc96136de85eb917178eba8218
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" path
measure: 4 cases of 1 twin directories, worst Xo / Go 1.136 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/path; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/path/clean | 6.5 | 6.3 | 0.974 |
| bench/perf/path/join | 17.6 | 19.6 | 1.109 |
| bench/perf/path/parts | 6.7 | 7.6 | 1.136 |
| bench/perf/path/match | 6.0 | 5.9 | 0.980 |
