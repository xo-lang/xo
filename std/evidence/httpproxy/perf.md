kind: perf
module: httpproxy
result: pass
commit: a6cabf508
source: 49b0fe33f2c3f3cafef3bad0993c8f0221695a000b61ca3950415d110117bfec
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" httpproxy
measure: 3 cases of 1 twin directories, worst Xo / Go 0.926 over the cases above 5 ms (Proven at or under 1.05), 16s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/httpproxy_rules; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/httpproxy_rules/none (under 5 ms: startup) | 0.7 | 0.6 | 0.811 |
| bench/perf/httpproxy_rules/route | 300.3 | 278.1 | 0.926 |
| bench/perf/httpproxy_rules/rules | 452.1 | 368.7 | 0.815 |
