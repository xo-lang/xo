kind: perf
module: udp
result: pass
commit: 117bb3891
source: 7617b2b96320ae08da7def65a0251b73d659a34ecabfca87a684e4ddd602401f
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" udp
measure: 2 cases of 1 twin directories, worst Xo / Go 0.977 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/udp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/udp/none (under 5 ms: startup) | 0.8 | 0.5 | 0.721 |
| bench/perf/udp/echo | 172.8 | 168.8 | 0.977 |
