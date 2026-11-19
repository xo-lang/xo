kind: perf
module: testing
result: pass
commit: 154bb8e67
source: 20c433552355904d373b4591bf1360691eaf5e629e4c8d1b81213271dbbcee7a
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" testing
measure: 1 cases of 1 twin directories, worst Xo / Go 0.129 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/testing; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/testing | 195.4 | 25.2 | 0.129 |
