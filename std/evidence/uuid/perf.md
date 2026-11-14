kind: perf
module: uuid
result: pass
commit: 4357973f3
source: 161bd8e78fb57c3730c333cb7e0cb76877ab871b58aea8106d31d54210b67fe9
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" uuid
measure: 4 cases of 1 twin directories, worst Xo / Go 0.924 over the cases above 5 ms (Proven at or under 1.05), 1s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/uuid; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/uuid/none (under 5 ms: startup) | 1.0 | 0.8 | 0.805 |
| bench/perf/uuid/parse | 11.6 | 10.8 | 0.924 |
| bench/perf/uuid/string | 7.9 | 5.6 | 0.703 |
| bench/perf/uuid/bytes (under 5 ms: startup) | 1.3 | 1.4 | 1.048 |
