kind: perf
module: multipart
result: pass
commit: eaa07eb3b
source: a0e54a10519963008a4324abf649a13fa554f1242b112d3669fef9fe6fa24155
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" multipart
measure: 3 cases of 1 twin directories, worst Xo / Go 0.489 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/multipart; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/multipart/none (under 5 ms: startup) | 0.8 | 0.5 | 0.672 |
| bench/perf/multipart/parse | 198.4 | 97.0 | 0.489 |
| bench/perf/multipart/write | 122.9 | 39.3 | 0.320 |
