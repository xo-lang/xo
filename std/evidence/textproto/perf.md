kind: perf
module: textproto
result: fail
commit: eaa07eb3b
source: a407978fe470b043182dfbe167d80c9c14150cbf69423ec56acb41bb7428a96d
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" textproto
measure: 4 cases of 1 twin directories, worst Xo / Go 1.308 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/textproto; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/textproto/none (under 5 ms: startup) | 0.7 | 0.5 | 0.721 |
| bench/perf/textproto/header | 184.4 | 241.3 | 1.308 |
| bench/perf/textproto/canonical | 83.0 | 104.3 | 1.257 |
| bench/perf/textproto/dot | 243.5 | 107.9 | 0.443 |
