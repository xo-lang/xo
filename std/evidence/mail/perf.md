kind: perf
module: mail
result: pass
commit: 117bb3891
source: 9a76befc4bd13f488966a94ef83bfc654a71bd6aedcdfb066a25120a0679b9c8
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" mail
measure: 6 cases of 1 twin directories, worst Xo / Go 1.032 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/mail; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/mail/none (under 5 ms: startup) | 0.8 | 0.8 | 0.941 |
| bench/perf/mail/addr | 36.4 | 20.9 | 0.574 |
| bench/perf/mail/list | 118.0 | 61.2 | 0.519 |
| bench/perf/mail/display | 11.4 | 11.8 | 1.032 |
| bench/perf/mail/msg | 120.4 | 51.4 | 0.427 |
| bench/perf/mail/date | 113.8 | 23.6 | 0.207 |
