kind: perf
module: regexp
result: fail
commit: 57d0505e3
source: 008efc3ad36ad39b63b18d87046acd8dc10667ce6ad26d1716e408e813180590
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" regexp
measure: 6 cases of 1 twin directories, worst Xo / Go 1.137 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/regexp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/regexp/none (under 5 ms: startup) | 1.1 | 0.8 | 0.736 |
| bench/perf/regexp/compile | 76.6 | 61.9 | 0.807 |
| bench/perf/regexp/match | 50.7 | 57.7 | 1.137 |
| bench/perf/regexp/find_all | 99.3 | 82.3 | 0.829 |
| bench/perf/regexp/replace_all | 76.5 | 70.2 | 0.918 |
| bench/perf/regexp/captures | 9.9 | 8.7 | 0.875 |
