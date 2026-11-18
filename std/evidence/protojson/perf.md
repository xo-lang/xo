kind: perf
module: protojson
result: fail
commit: b232ddf22
source: 8eb6aacf3f43230a357b7d8ec5afb336a926955d026156472b7ee69722fbb22e
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protojson
measure: 3 cases of 1 twin directories, worst Xo / Go 1.088 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protojson; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protojson/none (under 5 ms: startup) | 1.2 | 0.8 | 0.716 |
| bench/perf/protojson/marshal | 45.6 | 49.6 | 1.088 |
| bench/perf/protojson/unmarshal | 112.1 | 102.4 | 0.914 |
