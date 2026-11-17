kind: perf
module: protojson
result: fail
commit: e918cf3b5
source: 2a5032c94804667d04390998e26bcd1eedc65680d86004ce18b7c5fb3833fd6a
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protojson
measure: 3 cases of 1 twin directories, worst Xo / Go 1.099 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protojson; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protojson/none (under 5 ms: startup) | 1.2 | 0.8 | 0.710 |
| bench/perf/protojson/marshal | 43.6 | 48.0 | 1.099 |
| bench/perf/protojson/unmarshal | 108.5 | 95.4 | 0.880 |
