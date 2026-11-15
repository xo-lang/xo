kind: perf
module: cli
result: pass
commit: 374c25851
source: 06436b6976603b015b6652c833d7421c05016815a644e9e402eabcc9cad840d0
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" cli
measure: 2 cases of 1 twin directories, worst Xo / Go 0.924 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/cli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/cli/none (under 5 ms: startup) | 0.7 | 0.5 | 0.700 |
| bench/perf/cli/parse | 135.4 | 125.2 | 0.924 |
