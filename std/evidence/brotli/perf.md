kind: perf
module: brotli
result: fail
commit: 541463aee
source: 2b34f7d5a41fceb7224c1233e4fcd7d1fe30ed0ab0c20de29d997fb356eca13d
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" brotli
measure: 7 cases of 1 twin directories, worst Xo / Go 1.490 over the cases above 5 ms (Proven at or under 1.05), 15s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/brotli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/brotli/none (under 5 ms: startup) | 2.3 | 1.1 | 0.489 |
| bench/perf/brotli/text | 72.4 | 72.7 | 1.004 |
| bench/perf/brotli/html | 42.6 | 32.8 | 0.771 |
| bench/perf/brotli/mixed | 32.3 | 35.2 | 1.089 |
| bench/perf/brotli/enc1 | 58.4 | 47.7 | 0.816 |
| bench/perf/brotli/enc6 | 49.5 | 51.8 | 1.046 |
| bench/perf/brotli/enc11 | 241.4 | 359.6 | 1.490 |
