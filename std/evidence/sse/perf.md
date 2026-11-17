kind: perf
module: sse
result: pass
commit: c6629124b
source: 1281fd968ecaf1b00375e0c1a8c01461a637bae216ea5424d76be31cddf38f46
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" sse
measure: 3 cases of 1 twin directories, worst Xo / Go 1.025 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/sse; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/sse/none (under 5 ms: startup) | 0.8 | 0.5 | 0.658 |
| bench/perf/sse/encode | 18.3 | 16.6 | 0.907 |
| bench/perf/sse/parse | 127.7 | 130.9 | 1.025 |
