kind: perf
module: brotli
result: fail
commit: 2d5f4cb73
source: aec85fea18b592806d0200ba93dbf3bb81aefb3f79451436477cb37fc47e3b14
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" brotli
measure: 7 cases of 1 twin directories, worst Xo / Go 1.409 over the cases above 5 ms (Proven at or under 1.05), 15s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/brotli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/brotli/none (under 5 ms: startup) | 2.4 | 1.2 | 0.496 |
| bench/perf/brotli/text | 72.3 | 73.3 | 1.014 |
| bench/perf/brotli/html | 42.5 | 32.8 | 0.772 |
| bench/perf/brotli/mixed | 32.0 | 35.3 | 1.103 |
| bench/perf/brotli/enc1 | 57.9 | 46.7 | 0.807 |
| bench/perf/brotli/enc6 | 52.3 | 50.3 | 0.962 |
| bench/perf/brotli/enc11 | 242.0 | 341.0 | 1.409 |
