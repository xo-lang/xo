kind: perf
module: textproto
result: pass
commit: a2dcc9bc7
source: 3ee6c206038dafad864801f91bf39b968b19e5e2a161726cd9d33bc0552b0da8
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" textproto
measure: 4 cases of 1 twin directories, worst Xo / Go 1.040 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/textproto; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/textproto/none (under 5 ms: startup) | 0.7 | 0.5 | 0.749 |
| bench/perf/textproto/header | 188.5 | 196.1 | 1.040 |
| bench/perf/textproto/canonical | 80.3 | 77.3 | 0.963 |
| bench/perf/textproto/dot | 243.7 | 111.0 | 0.455 |
