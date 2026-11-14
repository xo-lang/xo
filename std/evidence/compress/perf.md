kind: perf
module: compress
result: pass
commit: 921ec3bc4
source: dc4120d24adfd08d11ffafde0be63dab3a5d2ba6c6cf8c7a38dd55aba245adf7
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" compress
measure: 6 cases of 1 twin directories, worst Xo / Go 0.948 over the cases above 5 ms (Proven at or under 1.05), 25s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/compress; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/compress/none (under 5 ms: startup) | 4.2 | 3.0 | 0.702 |
| bench/perf/compress/gzip | 493.4 | 461.5 | 0.935 |
| bench/perf/compress/gzip1 | 93.4 | 64.9 | 0.695 |
| bench/perf/compress/gunzip | 88.0 | 80.3 | 0.913 |
| bench/perf/compress/deflate | 483.8 | 458.8 | 0.948 |
| bench/perf/compress/inflate | 85.7 | 77.4 | 0.904 |
