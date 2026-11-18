kind: perf
module: brotli
result: fail
commit: 09f097a2d
source: f2c0bd89b9990914f9f1778da141df3967471bfa2baac385b47cc2276a814ca0
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" brotli
measure: 7 cases of 1 twin directories, worst Xo / Go 1.645 over the cases above 5 ms (Proven at or under 1.05), 15s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/brotli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/brotli/none (under 5 ms: startup) | 2.3 | 1.2 | 0.521 |
| bench/perf/brotli/text | 72.8 | 89.1 | 1.224 |
| bench/perf/brotli/html | 42.1 | 39.4 | 0.934 |
| bench/perf/brotli/mixed | 33.1 | 42.1 | 1.272 |
| bench/perf/brotli/enc1 | 59.9 | 53.2 | 0.887 |
| bench/perf/brotli/enc6 | 52.7 | 52.1 | 0.989 |
| bench/perf/brotli/enc11 | 241.4 | 397.0 | 1.645 |
