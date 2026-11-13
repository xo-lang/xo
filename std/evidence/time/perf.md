kind: perf
module: time
result: fail
commit: bbf1d4c6a
source: 6b464a82e4dcf536b562bb8996d17e1f319790b1d54201460d49d983fe157eec
date: 2026-11-13
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" time
measure: 10 cases of 2 twin directories, worst Xo / Go 1.510 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/time, bench/perf/timego; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/time/none (under 5 ms: startup) | 0.7 | 0.5 | 0.713 |
| bench/perf/time/format | 35.2 | 51.5 | 1.463 |
| bench/perf/time/parse | 18.0 | 21.2 | 1.174 |
| bench/perf/time/civil | 21.9 | 9.6 | 0.437 |
| bench/perf/time/duration | 14.7 | 10.6 | 0.723 |
| bench/perf/time/format_layout | 35.4 | 44.0 | 1.242 |
| bench/perf/time/parse_layout | 18.2 | 22.2 | 1.218 |
| bench/perf/timego/none (under 5 ms: startup) | 0.7 | 0.5 | 0.781 |
| bench/perf/timego/format | 33.4 | 44.7 | 1.337 |
| bench/perf/timego/parse | 9.1 | 13.7 | 1.510 |
