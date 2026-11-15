kind: perf
module: bufio
result: pass
commit: 117bb3891
source: 04a57004815927bde265ee90d0669b7630a75d58d2e9de7f28f6989ce282746a
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" bufio
measure: 4 cases of 1 twin directories, worst Xo / Go 0.670 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/bufio; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/bufio/none | 29.4 | 17.3 | 0.586 |
| bench/perf/bufio/lines | 35.7 | 23.9 | 0.670 |
| bench/perf/bufio/until | 47.4 | 27.9 | 0.588 |
| bench/perf/bufio/exact | 31.1 | 18.9 | 0.608 |
