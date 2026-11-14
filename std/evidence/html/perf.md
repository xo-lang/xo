kind: perf
module: html
result: pass
commit: 4357973f3
source: 0cc505576e1f0622c0cd2e3605368377c0fd7839e2d090ae6c2851976f217da3
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" html
measure: 6 cases of 2 twin directories, worst Xo / Go 0.948 over the cases above 5 ms (Proven at or under 1.05), 46s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/html, bench/perf/htmltemplate; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/html/none (under 5 ms: startup) | 0.7 | 0.5 | 0.706 |
| bench/perf/html/escape | 122.9 | 116.6 | 0.948 |
| bench/perf/html/unescape | 284.7 | 151.9 | 0.534 |
| bench/perf/htmltemplate/none (under 5 ms: startup) | 0.9 | 0.6 | 0.636 |
| bench/perf/htmltemplate/parse | 525.3 | 369.7 | 0.704 |
| bench/perf/htmltemplate/render | 1679.2 | 1182.9 | 0.704 |
