kind: perf
module: smtp
result: pass
commit: 117bb3891
source: 482405e7b0bf612407269458b71170ce8c795b831d2aa6bb4605ce2d8248366f
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" smtp
measure: 2 cases of 1 twin directories, worst Xo / Go 0.668 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/smtp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/smtp/none (under 5 ms: startup) | 0.8 | 0.5 | 0.664 |
| bench/perf/smtp/send | 184.4 | 123.1 | 0.668 |
