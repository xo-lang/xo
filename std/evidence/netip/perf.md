kind: perf
module: netip
result: pass
commit: 541463aee
source: 1a59b19fc612328095d6f6eea1dae91dc644feea4557db49a426aa34e25a5f3b
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" netip
measure: 7 cases of 1 twin directories, worst Xo / Go 0.960 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/netip; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/netip/none (under 5 ms: startup) | 1.5 | 1.1 | 0.739 |
| bench/perf/netip/mixed | 21.7 | 16.9 | 0.781 |
| bench/perf/netip/parse4 | 6.2 | 5.8 | 0.938 |
| bench/perf/netip/parse6 | 10.0 | 8.3 | 0.832 |
| bench/perf/netip/display4 | 10.2 | 6.2 | 0.609 |
| bench/perf/netip/display6 | 20.6 | 11.2 | 0.542 |
| bench/perf/netip/compare | 23.0 | 22.1 | 0.960 |
