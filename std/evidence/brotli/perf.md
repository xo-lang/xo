kind: perf
module: brotli
result: fail
commit: ab1738233
source: 0678a87bf2580df62852f64dd9df35d8d0f9da19390731b7a00832e1d9bd7443
date: 2026-11-20
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" brotli
measure: 7 cases of 1 twin directories, worst Xo / Go 1.364 over the cases above 5 ms (Proven at or under 1.05), 11s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/brotli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/brotli/none (under 5 ms: startup) | 0.9 | 0.5 | 0.553 |
| bench/perf/brotli/text | 53.9 | 54.8 | 1.017 |
| bench/perf/brotli/html | 24.1 | 24.2 | 1.008 |
| bench/perf/brotli/mixed | 24.9 | 21.9 | 0.879 |
| bench/perf/brotli/enc1 | 24.6 | 17.5 | 0.709 |
| bench/perf/brotli/enc6 | 23.5 | 27.4 | 1.166 |
| bench/perf/brotli/enc11 | 181.8 | 247.9 | 1.364 |
