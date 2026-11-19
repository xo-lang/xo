kind: perf
module: brotli
result: fail
commit: 3a80fdd33
source: 7c94400515e31a2a635d1bd53f6933e995406bbcb892d9645c7c279937329816
date: 2026-11-19
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" brotli
measure: 7 cases of 1 twin directories, worst Xo / Go 1.555 over the cases above 5 ms (Proven at or under 1.05), 11s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/brotli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/brotli/none (under 5 ms: startup) | 0.7 | 0.4 | 0.558 |
| bench/perf/brotli/text | 50.5 | 55.4 | 1.098 |
| bench/perf/brotli/html | 23.9 | 25.4 | 1.065 |
| bench/perf/brotli/mixed | 24.3 | 25.4 | 1.044 |
| bench/perf/brotli/enc1 | 25.8 | 36.1 | 1.401 |
| bench/perf/brotli/enc6 | 21.7 | 32.7 | 1.510 |
| bench/perf/brotli/enc11 | 179.6 | 279.4 | 1.555 |
