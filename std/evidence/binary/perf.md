kind: perf
module: binary
result: pass
commit: 117bb3891
source: b1ef0316227b0da7f955a50d989db596dc6b5bd32a4dc947b60dd076f3069803
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" binary
measure: 3 cases of 1 twin directories, worst Xo / Go 0.881 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/binary; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/binary/none (under 5 ms: startup) | 1.7 | 1.1 | 0.639 |
| bench/perf/binary/fixed | 32.0 | 24.1 | 0.754 |
| bench/perf/binary/varint | 28.8 | 25.4 | 0.881 |
