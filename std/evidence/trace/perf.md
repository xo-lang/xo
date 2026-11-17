kind: perf
module: trace
result: fail
commit: a2dcc9bc7
source: f38e98426a44dfc3b49caeed9c4da32db8b9ab35e8812e6f9edaba7ea45c1294
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" trace
measure: 4 cases of 1 twin directories, worst Xo / Go 1.210 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/trace; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/trace/none (under 5 ms: startup) | 3.6 | 3.0 | 0.824 |
| bench/perf/trace/record | 20.5 | 24.9 | 1.210 |
| bench/perf/trace/span | 19.3 | 21.0 | 1.089 |
| bench/perf/trace/read | 9.8 | 4.5 | 0.456 |
