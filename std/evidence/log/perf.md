kind: perf
module: log
result: fail
commit: 09f097a2d
source: 356912ac7a981274310b0ff6ca538cd4bb67c35c16058df7391e74fe5eb99574
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" log
measure: 4 cases of 1 twin directories, worst Xo / Go 1.068 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/log; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/log/none (under 5 ms: startup) | 1.7 | 1.7 | 0.981 |
| bench/perf/log/filtered | 9.7 | 1.3 | 0.136 |
| bench/perf/log/text | 131.8 | 117.7 | 0.893 |
| bench/perf/log/json | 123.2 | 131.5 | 1.068 |
