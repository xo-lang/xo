kind: perf
module: migrate
result: pass
commit: 8b0b348a0
source: 527c599689b33b4fea3c7ea450efa9bbd21ec525c3699d9272a437e56cf1bc9d
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" migrate
measure: 2 cases of 1 twin directories, worst Xo / Go 0.564 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/migrate; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/migrate/none (under 5 ms: startup) | 1.5 | 1.1 | 0.703 |
| bench/perf/migrate/parse | 324.7 | 183.2 | 0.564 |
