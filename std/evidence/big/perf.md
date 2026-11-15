kind: perf
module: big
result: fail
commit: 117bb3891
source: f92f22e49dda19676f6f6547e0f1353b8a6f430e2eb33d11ba737fb4f7840aee
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" big
measure: 21 cases of 3 twin directories, worst Xo / Go 1.460 over the cases above 5 ms (Proven at or under 1.05), 17s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/big, bench/perf/bigfloat, bench/perf/bigrat; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/big/none (under 5 ms: startup) | 0.6 | 0.6 | 0.889 |
| bench/perf/big/mul | 8.5 | 8.5 | 0.998 |
| bench/perf/big/quorem | 9.2 | 9.0 | 0.976 |
| bench/perf/big/text | 5.0 | 5.6 | 1.114 |
| bench/perf/big/parse | 7.0 | 3.2 | 0.462 |
| bench/perf/big/exp | 57.8 | 47.1 | 0.814 |
| bench/perf/bigfloat/none (under 5 ms: startup) | 0.6 | 0.5 | 0.837 |
| bench/perf/bigfloat/add | 11.2 | 11.5 | 1.021 |
| bench/perf/bigfloat/mul | 14.7 | 13.9 | 0.946 |
| bench/perf/bigfloat/quo | 22.6 | 23.3 | 1.031 |
| bench/perf/bigfloat/sqrt | 10.9 | 14.2 | 1.303 |
| bench/perf/bigfloat/text | 36.9 | 37.0 | 1.001 |
| bench/perf/bigfloat/parse | 20.4 | 14.1 | 0.689 |
| bench/perf/bigrat/none (under 5 ms: startup) | 0.7 | 0.5 | 0.747 |
| bench/perf/bigrat/add | 147.1 | 135.4 | 0.920 |
| bench/perf/bigrat/mul | 254.3 | 225.6 | 0.887 |
| bench/perf/bigrat/quo | 65.8 | 61.7 | 0.938 |
| bench/perf/bigrat/cmp | 21.9 | 26.7 | 1.221 |
| bench/perf/bigrat/tofloat | 33.4 | 27.9 | 0.834 |
| bench/perf/bigrat/text | 18.6 | 27.2 | 1.460 |
| bench/perf/bigrat/parse | 50.2 | 40.8 | 0.812 |
