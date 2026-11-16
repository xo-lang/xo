kind: perf
module: big
result: fail
commit: ce0a8ffb1
source: 9629a1013fb3a901979446fb0568a2fbe2ee1a3caa082d0614abbf1e9f64f7bb
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" big
measure: 21 cases of 3 twin directories, worst Xo / Go 1.262 over the cases above 5 ms (Proven at or under 1.05), 18s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/big, bench/perf/bigfloat, bench/perf/bigrat; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/big/none (under 5 ms: startup) | 0.7 | 0.6 | 0.852 |
| bench/perf/big/mul | 8.6 | 8.8 | 1.022 |
| bench/perf/big/quorem | 9.5 | 10.0 | 1.058 |
| bench/perf/big/text | 5.2 | 5.5 | 1.067 |
| bench/perf/big/parse | 7.3 | 3.5 | 0.483 |
| bench/perf/big/exp | 57.8 | 46.9 | 0.811 |
| bench/perf/bigfloat/none (under 5 ms: startup) | 0.6 | 0.5 | 0.750 |
| bench/perf/bigfloat/add | 11.5 | 12.4 | 1.085 |
| bench/perf/bigfloat/mul | 14.9 | 18.8 | 1.262 |
| bench/perf/bigfloat/quo | 23.2 | 20.9 | 0.898 |
| bench/perf/bigfloat/sqrt | 10.8 | 9.5 | 0.879 |
| bench/perf/bigfloat/text | 39.1 | 37.8 | 0.965 |
| bench/perf/bigfloat/parse | 21.0 | 14.7 | 0.699 |
| bench/perf/bigrat/none (under 5 ms: startup) | 0.6 | 0.5 | 0.783 |
| bench/perf/bigrat/add | 156.9 | 146.2 | 0.932 |
| bench/perf/bigrat/mul | 270.1 | 244.7 | 0.906 |
| bench/perf/bigrat/quo | 68.9 | 64.5 | 0.937 |
| bench/perf/bigrat/cmp | 21.3 | 1.8 | 0.085 |
| bench/perf/bigrat/tofloat | 34.0 | 29.1 | 0.858 |
| bench/perf/bigrat/text | 18.3 | 17.5 | 0.959 |
| bench/perf/bigrat/parse | 51.5 | 43.7 | 0.850 |
