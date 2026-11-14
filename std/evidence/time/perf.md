kind: perf
module: time
result: pass
commit: a0c61aef6
source: 615bb2e80abc81996374c87057a298c015893ddcb4f2d2118bbf142de8e94fba
date: 2026-11-14
host: linux/arm64
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64" time
measure: 10 cases of 2 twin directories, worst Xo / Go 0.995 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/time, bench/perf/timego; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/time/none (under 5 ms: startup) | 0.7 | 0.5 | 0.796 |
| bench/perf/time/format | 35.7 | 31.8 | 0.891 |
| bench/perf/time/parse | 18.3 | 10.1 | 0.553 |
| bench/perf/time/civil | 21.9 | 8.3 | 0.381 |
| bench/perf/time/duration | 14.8 | 11.0 | 0.744 |
| bench/perf/time/format_layout | 35.3 | 22.7 | 0.644 |
| bench/perf/time/parse_layout | 17.8 | 9.2 | 0.519 |
| bench/perf/timego/none (under 5 ms: startup) | 0.7 | 0.5 | 0.759 |
| bench/perf/timego/format | 33.1 | 33.0 | 0.995 |
| bench/perf/timego/parse | 9.1 | 8.2 | 0.908 |
