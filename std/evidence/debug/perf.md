kind: perf
module: debug
result: pass
commit: 09f097a2d
source: f6f6aa8764de034afc29c9c0670a05edadd55005ac187836ce5d5a56ba50ecb3
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" debug
measure: 4 cases of 1 twin directories, worst Xo / Go 0.896 over the cases above 5 ms (Proven at or under 1.05), 21s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/debug; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/debug/none (under 5 ms: startup) | 2.3 | 1.2 | 0.518 |
| bench/perf/debug/refused | 538.2 | 106.9 | 0.199 |
| bench/perf/debug/sched | 521.8 | 187.5 | 0.359 |
| bench/perf/debug/mem | 269.2 | 241.3 | 0.896 |
