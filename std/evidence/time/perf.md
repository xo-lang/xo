kind: perf
module: time
result: pass
commit: cde48f697
source: b179c89e3ae58e0d68d5883cdb26c3b8a7f2a8b8c5ac3ea5e505915154dfd398
date: 2026-11-14
host: linux/arm64
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64" time
measure: 10 cases of 2 twin directories, worst Xo / Go 0.952 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/time, bench/perf/timego; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/time/none (under 5 ms: startup) | 0.6 | 0.5 | 0.716 |
| bench/perf/time/format | 35.8 | 32.3 | 0.903 |
| bench/perf/time/parse | 18.3 | 10.0 | 0.547 |
| bench/perf/time/civil | 22.0 | 8.3 | 0.377 |
| bench/perf/time/duration | 14.9 | 11.0 | 0.737 |
| bench/perf/time/format_layout | 35.3 | 24.6 | 0.697 |
| bench/perf/time/parse_layout | 18.1 | 9.2 | 0.508 |
| bench/perf/timego/none (under 5 ms: startup) | 0.5 | 0.5 | 0.914 |
| bench/perf/timego/format | 33.0 | 31.4 | 0.952 |
| bench/perf/timego/parse | 8.9 | 8.1 | 0.907 |
