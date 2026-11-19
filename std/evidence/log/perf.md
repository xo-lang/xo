kind: perf
module: log
result: pass
commit: 541463aee
source: f400632946f861a96c08a3911fd73722e5ccfb569edb0c81f14f1f7eef787748
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" log
measure: 4 cases of 1 twin directories, worst Xo / Go 0.982 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/log; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/log/none (under 5 ms: startup) | 1.7 | 1.6 | 0.968 |
| bench/perf/log/filtered | 9.2 | 1.4 | 0.155 |
| bench/perf/log/text | 131.5 | 117.4 | 0.893 |
| bench/perf/log/json | 123.1 | 120.9 | 0.982 |
