kind: perf
module: netip
result: fail
commit: 117bb3891
source: 671d33a1a96fac66ad46542b18a37661e505704275d3fc6fa5d5a6ef3d41a238
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" netip
measure: 7 cases of 1 twin directories, worst Xo / Go 1.407 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/netip; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/netip/none (under 5 ms: startup) | 0.6 | 0.5 | 0.765 |
| bench/perf/netip/mixed | 19.6 | 22.0 | 1.120 |
| bench/perf/netip/parse4 | 5.3 | 5.9 | 1.112 |
| bench/perf/netip/parse6 | 11.1 | 15.6 | 1.407 |
| bench/perf/netip/display4 | 6.8 | 5.6 | 0.821 |
| bench/perf/netip/display6 | 15.6 | 11.0 | 0.701 |
| bench/perf/netip/compare | 18.0 | 17.2 | 0.959 |
