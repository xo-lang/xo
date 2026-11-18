kind: perf
module: netip
result: fail
commit: 09f097a2d
source: 1a59b19fc612328095d6f6eea1dae91dc644feea4557db49a426aa34e25a5f3b
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" netip
measure: 7 cases of 1 twin directories, worst Xo / Go 1.323 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/netip; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/netip/none (under 5 ms: startup) | 1.5 | 1.1 | 0.745 |
| bench/perf/netip/mixed | 21.7 | 16.3 | 0.751 |
| bench/perf/netip/parse4 | 6.4 | 5.8 | 0.905 |
| bench/perf/netip/parse6 | 10.3 | 8.3 | 0.808 |
| bench/perf/netip/display4 | 10.0 | 6.2 | 0.625 |
| bench/perf/netip/display6 | 21.0 | 10.0 | 0.477 |
| bench/perf/netip/compare | 23.0 | 30.4 | 1.323 |
