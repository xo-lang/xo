kind: perf
module: log
result: fail
commit: 57d0505e3
source: 9980e5649ada9d0f34ff184ecc55d0b585a40fcb83e64b1e46c07b95fe4c6c04
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" log
measure: 4 cases of 1 twin directories, worst Xo / Go 1.204 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/log; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/log/none (under 5 ms: startup) | 0.7 | 0.6 | 0.818 |
| bench/perf/log/filtered | 7.4 | 0.5 | 0.073 |
| bench/perf/log/text | 97.2 | 85.3 | 0.878 |
| bench/perf/log/json | 90.9 | 109.5 | 1.204 |
