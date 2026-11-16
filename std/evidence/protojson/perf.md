kind: perf
module: protojson
result: fail
commit: 536b01c2b
source: 3bc40ebc711db9596253148f9afb8e56faa95d24ca6391761b21c66faf3812d0
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protojson
measure: 3 cases of 1 twin directories, worst Xo / Go 1.642 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protojson; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protojson/none (under 5 ms: startup) | 1.2 | 1.0 | 0.865 |
| bench/perf/protojson/marshal | 46.1 | 75.6 | 1.642 |
| bench/perf/protojson/unmarshal | 109.9 | 95.5 | 0.869 |
