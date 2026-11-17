kind: perf
module: log
result: fail
commit: 815bba658
source: 356912ac7a981274310b0ff6ca538cd4bb67c35c16058df7391e74fe5eb99574
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" log
measure: 4 cases of 1 twin directories, worst Xo / Go 1.163 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/log; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/log/none (under 5 ms: startup) | 0.8 | 0.6 | 0.799 |
| bench/perf/log/filtered | 7.4 | 0.6 | 0.078 |
| bench/perf/log/text | 99.2 | 85.8 | 0.865 |
| bench/perf/log/json | 91.6 | 106.5 | 1.163 |
