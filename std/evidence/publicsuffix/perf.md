kind: perf
module: publicsuffix
result: fail
commit: 3f0e5e79d
source: 79924efc0afde9376828c4d52c2308988b74ee1ab4755b2407b6ef20072cc242
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" publicsuffix
measure: 3 cases of 1 twin directories, worst Xo / Go 1.566 over the cases above 5 ms (Proven at or under 1.05), 19s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/publicsuffix; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/publicsuffix/none (under 5 ms: startup) | 0.8 | 0.5 | 0.704 |
| bench/perf/publicsuffix/suffix | 302.8 | 474.2 | 1.566 |
| bench/perf/publicsuffix/etld1 | 427.6 | 569.8 | 1.333 |
