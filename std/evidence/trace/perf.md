kind: perf
module: trace
result: pass
commit: b232ddf22
source: 0ef9354062a5b3582dbef70c2eaad8f6ab7336b185a6322a16a3a254ff32dae8
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" trace
measure: 4 cases of 1 twin directories, worst Xo / Go 0.967 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/trace; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/trace/none (under 5 ms: startup) | 3.6 | 3.0 | 0.823 |
| bench/perf/trace/record | 20.4 | 15.1 | 0.741 |
| bench/perf/trace/span | 20.3 | 19.6 | 0.967 |
| bench/perf/trace/read | 9.9 | 4.7 | 0.477 |
