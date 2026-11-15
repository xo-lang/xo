kind: perf
module: xml
result: pass
commit: 7645bf7aa
source: 4fb9170951174d09c02628a1a0a4f3abf1b958d2d238cd534f341ff42f329931
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" xml
measure: 4 cases of 1 twin directories, worst Xo / Go 0.490 over the cases above 5 ms (Proven at or under 1.05), 12s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/xml; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/xml/none (under 5 ms: startup) | 2.9 | 1.3 | 0.445 |
| bench/perf/xml/tokens | 209.5 | 101.9 | 0.487 |
| bench/perf/xml/stream | 210.1 | 102.9 | 0.490 |
| bench/perf/xml/tree | 296.2 | 127.2 | 0.430 |
