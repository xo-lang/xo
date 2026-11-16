kind: perf
module: protodesc
result: pass
commit: 536b01c2b
source: 23ea718fe093b340f07d0d332ce773237e2507f9a780e70bbd842de959598bd6
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protodesc
measure: 3 cases of 1 twin directories, worst Xo / Go 0.959 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protodesc; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protodesc/none (under 5 ms: startup) | 0.8 | 0.5 | 0.620 |
| bench/perf/protodesc/decode | 108.7 | 79.9 | 0.735 |
| bench/perf/protodesc/encode | 48.2 | 46.3 | 0.959 |
