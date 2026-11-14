kind: perf
module: compress
result: pass
commit: 451133bcd
source: c6e28c6b292bd1f66102119f96c9c8a5f26d9474c9ea128482417979e5dba89d
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" compress
measure: 6 cases of 1 twin directories, worst Xo / Go 0.953 over the cases above 5 ms (Proven at or under 1.05), 26s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/compress; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/compress/none (under 5 ms: startup) | 4.2 | 3.0 | 0.720 |
| bench/perf/compress/gzip | 489.5 | 461.2 | 0.942 |
| bench/perf/compress/gzip1 | 97.6 | 64.7 | 0.664 |
| bench/perf/compress/gunzip | 89.0 | 80.3 | 0.902 |
| bench/perf/compress/deflate | 481.7 | 458.9 | 0.953 |
| bench/perf/compress/inflate | 88.1 | 78.5 | 0.891 |
