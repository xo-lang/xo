kind: perf
module: norm
result: pass
commit: 1d09c6220
source: 2bcf53a513f181d80ee492f3bbda04e23d9091a43a8ee5414aca65937bff9f63
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" norm
measure: 5 cases of 1 twin directories, worst Xo / Go 0.942 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/norm; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/norm/none (under 5 ms: startup) | 4.0 | 3.6 | 0.897 |
| bench/perf/norm/ascii | 5.8 | 5.1 | 0.872 |
| bench/perf/norm/nfc | 92.0 | 57.0 | 0.620 |
| bench/perf/norm/nfd | 123.1 | 115.9 | 0.942 |
| bench/perf/norm/nfkc | 133.5 | 86.1 | 0.645 |
