kind: perf
module: norm
result: pass
commit: 4357973f3
source: 489994737e62efc2a4c8203eafcc57ba75119a6bb6f7f3ccc9469578cd688982
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" norm
measure: 5 cases of 1 twin directories, worst Xo / Go 0.946 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/norm; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/norm/none (under 5 ms: startup) | 4.0 | 3.3 | 0.830 |
| bench/perf/norm/ascii | 5.9 | 4.6 | 0.790 |
| bench/perf/norm/nfc | 84.7 | 53.9 | 0.637 |
| bench/perf/norm/nfd | 115.4 | 109.2 | 0.946 |
| bench/perf/norm/nfkc | 127.0 | 81.3 | 0.640 |
