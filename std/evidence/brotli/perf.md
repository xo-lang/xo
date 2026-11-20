kind: perf
module: brotli
result: pass
commit: 41b9e57df
source: bd7fc2675dd60b038eb98c9c5740c02dbfc6d55ac0b9fce3851abc4bb6a97096
date: 2026-11-20
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" brotli
measure: 7 cases of 1 twin directories, worst Xo / Go 1.024 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/brotli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/brotli/none (under 5 ms: startup) | 0.7 | 0.4 | 0.532 |
| bench/perf/brotli/text | 51.4 | 48.4 | 0.942 |
| bench/perf/brotli/html | 24.4 | 23.1 | 0.946 |
| bench/perf/brotli/mixed | 24.5 | 21.5 | 0.876 |
| bench/perf/brotli/enc1 | 34.1 | 16.1 | 0.472 |
| bench/perf/brotli/enc6 | 21.9 | 20.4 | 0.930 |
| bench/perf/brotli/enc11 | 172.0 | 176.1 | 1.024 |
