kind: perf
module: log
result: fail
commit: 117bb3891
source: 4ff0b817780cbd8ac7e54f46b8081285e673c523f1396fa9828134ecf28c2321
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" log
measure: 4 cases of 1 twin directories, worst Xo / Go 1.184 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/log; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/log/none (under 5 ms: startup) | 0.7 | 0.6 | 0.862 |
| bench/perf/log/filtered | 7.3 | 0.6 | 0.078 |
| bench/perf/log/text | 99.8 | 70.9 | 0.710 |
| bench/perf/log/json | 92.4 | 109.5 | 1.184 |
