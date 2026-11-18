kind: perf
module: protojson
result: pass
commit: 09f097a2d
source: 8eb6aacf3f43230a357b7d8ec5afb336a926955d026156472b7ee69722fbb22e
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" protojson
measure: 3 cases of 1 twin directories, worst Xo / Go 0.934 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protojson; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protojson/none (under 5 ms: startup) | 2.5 | 1.6 | 0.633 |
| bench/perf/protojson/marshal | 64.8 | 60.6 | 0.934 |
| bench/perf/protojson/unmarshal | 135.7 | 111.4 | 0.821 |
