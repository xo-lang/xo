kind: perf
module: regexp
result: pass
commit: 815bba658
source: d7416328dfd79e71fe92a0334cc6550a1b0207cca9e6dcc3b7df79ab1c236131
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" regexp
measure: 6 cases of 1 twin directories, worst Xo / Go 0.998 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/regexp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/regexp/none (under 5 ms: startup) | 0.9 | 0.8 | 0.826 |
| bench/perf/regexp/compile | 77.7 | 60.0 | 0.772 |
| bench/perf/regexp/match | 51.0 | 51.0 | 0.998 |
| bench/perf/regexp/find_all | 99.3 | 82.6 | 0.832 |
| bench/perf/regexp/replace_all | 73.6 | 70.6 | 0.958 |
| bench/perf/regexp/captures | 10.0 | 8.3 | 0.829 |
