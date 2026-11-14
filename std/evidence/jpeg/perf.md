kind: perf
module: jpeg
result: pass
commit: 5a23d3a84
source: 7fa1007c418147a9ece3d6e9ef541267fbdb9c35e9e94dc2e8b22d5e36fb8ee5
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" jpeg
measure: 5 cases of 1 twin directories, worst Xo / Go 1.022 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/jpeg; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/jpeg/decode | 90.3 | 91.3 | 1.011 |
| bench/perf/jpeg/rgba | 101.9 | 104.1 | 1.022 |
| bench/perf/jpeg/none (under 5 ms: startup) | 0.6 | 0.5 | 0.845 |
| bench/perf/jpeg/encode | 67.6 | 58.3 | 0.862 |
| bench/perf/jpeg/encrgba | 76.3 | 70.5 | 0.925 |
