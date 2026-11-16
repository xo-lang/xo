kind: perf
module: sse
result: pass
commit: 65e891635
source: 3ee00bd627eb7d28d94fb5408f6d044e02052c083f3e24cdd1e9573094948af3
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" sse
measure: 3 cases of 1 twin directories, worst Xo / Go 0.974 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/sse; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/sse/none (under 5 ms: startup) | 0.8 | 0.5 | 0.683 |
| bench/perf/sse/encode | 18.9 | 16.7 | 0.887 |
| bench/perf/sse/parse | 128.9 | 125.5 | 0.974 |
