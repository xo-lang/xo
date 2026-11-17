kind: perf
module: log
result: fail
commit: 502a0170c
source: 0eded0eb0c05714b12c957da87f10ddca99d57b35510b45c40d51027bab994bb
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" log
measure: 4 cases of 1 twin directories, worst Xo / Go 1.537 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/log; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/log/none (under 5 ms: startup) | 0.8 | 0.6 | 0.754 |
| bench/perf/log/filtered | 7.5 | 0.6 | 0.073 |
| bench/perf/log/text | 99.0 | 87.9 | 0.888 |
| bench/perf/log/json | 92.8 | 142.7 | 1.537 |
