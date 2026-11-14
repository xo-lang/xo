kind: perf
module: html
result: pass
commit: 5a23d3a84
source: 5747b639da584a7ec48020992f3b0645191623cb176c4d06ef908f4d699be444
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" html
measure: 6 cases of 2 twin directories, worst Xo / Go 0.957 over the cases above 5 ms (Proven at or under 1.05), 46s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/html, bench/perf/htmltemplate; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/html/none (under 5 ms: startup) | 0.7 | 0.5 | 0.705 |
| bench/perf/html/escape | 122.7 | 117.4 | 0.957 |
| bench/perf/html/unescape | 287.0 | 135.8 | 0.473 |
| bench/perf/htmltemplate/none (under 5 ms: startup) | 0.9 | 0.5 | 0.624 |
| bench/perf/htmltemplate/parse | 532.1 | 370.8 | 0.697 |
| bench/perf/htmltemplate/render | 1728.1 | 1192.4 | 0.690 |
