kind: perf
module: url
result: pass
commit: 4357973f3
source: 828c07e20050977f91ae4bedd1c8a18ed0bfc2a4e83f6bf20f743cf38fda169f
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" url
measure: 5 cases of 1 twin directories, worst Xo / Go 1.002 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/url; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/url/none (under 5 ms: startup) | 0.9 | 0.8 | 0.847 |
| bench/perf/url/escape | 12.1 | 12.1 | 1.002 |
| bench/perf/url/unescape | 19.6 | 17.4 | 0.886 |
| bench/perf/url/query | 33.1 | 31.0 | 0.936 |
| bench/perf/url/encode | 67.9 | 52.1 | 0.768 |
