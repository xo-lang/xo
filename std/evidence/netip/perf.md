kind: perf
module: netip
result: fail
commit: 502a0170c
source: 6aeea21f62d1abfcd2937ac1d0793408dede9586a9305b2d1c3514244e07034b
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" netip
measure: 7 cases of 1 twin directories, worst Xo / Go 1.517 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/netip; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/netip/none (under 5 ms: startup) | 0.7 | 0.5 | 0.714 |
| bench/perf/netip/mixed | 20.7 | 24.5 | 1.185 |
| bench/perf/netip/parse4 | 5.5 | 6.7 | 1.214 |
| bench/perf/netip/parse6 | 11.5 | 17.5 | 1.517 |
| bench/perf/netip/display4 | 7.0 | 6.1 | 0.860 |
| bench/perf/netip/display6 | 16.5 | 12.1 | 0.737 |
| bench/perf/netip/compare | 18.0 | 17.8 | 0.993 |
