kind: perf
module: netip
result: fail
commit: b232ddf22
source: 1a59b19fc612328095d6f6eea1dae91dc644feea4557db49a426aa34e25a5f3b
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" netip
measure: 7 cases of 1 twin directories, worst Xo / Go 1.203 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/netip; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/netip/none (under 5 ms: startup) | 0.6 | 0.5 | 0.851 |
| bench/perf/netip/mixed | 20.7 | 20.8 | 1.007 |
| bench/perf/netip/parse4 | 5.4 | 6.5 | 1.203 |
| bench/perf/netip/parse6 | 11.6 | 8.7 | 0.750 |
| bench/perf/netip/display4 | 7.0 | 6.5 | 0.919 |
| bench/perf/netip/display6 | 16.2 | 11.6 | 0.716 |
| bench/perf/netip/compare | 18.7 | 17.9 | 0.955 |
