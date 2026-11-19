kind: perf
module: ui
result: pass
commit: 29712b0f5
source: 7c8e62bbbfa0f7e4604cf837272030289e16b8fa20c04f152319f1c5b2868dad
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" ui
measure: 3 cases of 1 twin directories, worst Xo / Go 0.576 over the cases above 5 ms (Proven at or under 1.05), 12s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/ui; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/ui/none (under 5 ms: startup) | 1.8 | 1.3 | 0.762 |
| bench/perf/ui/render | 472.2 | 271.9 | 0.576 |
| bench/perf/ui/build | 141.8 | 31.0 | 0.218 |
