kind: perf
module: big
result: fail
commit: 374c25851
source: 328f96d92cb7ff19082a66b08cd297139cb361a26c05883fea33218d1bd76e69
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" big
measure: 21 cases of 3 twin directories, worst Xo / Go 1.334 over the cases above 5 ms (Proven at or under 1.05), 18s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/big, bench/perf/bigfloat, bench/perf/bigrat; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/big/none (under 5 ms: startup) | 0.7 | 0.5 | 0.815 |
| bench/perf/big/mul | 8.7 | 8.9 | 1.023 |
| bench/perf/big/quorem | 9.6 | 9.7 | 1.007 |
| bench/perf/big/text | 5.2 | 5.8 | 1.107 |
| bench/perf/big/parse | 7.4 | 3.2 | 0.433 |
| bench/perf/big/exp | 57.9 | 47.1 | 0.813 |
| bench/perf/bigfloat/none (under 5 ms: startup) | 0.7 | 0.5 | 0.776 |
| bench/perf/bigfloat/add | 11.7 | 11.9 | 1.018 |
| bench/perf/bigfloat/mul | 15.0 | 14.9 | 0.988 |
| bench/perf/bigfloat/quo | 23.4 | 19.9 | 0.851 |
| bench/perf/bigfloat/sqrt | 11.2 | 14.9 | 1.334 |
| bench/perf/bigfloat/text | 38.2 | 37.0 | 0.969 |
| bench/perf/bigfloat/parse | 20.9 | 14.6 | 0.700 |
| bench/perf/bigrat/none (under 5 ms: startup) | 0.6 | 0.5 | 0.804 |
| bench/perf/bigrat/add | 153.4 | 143.1 | 0.933 |
| bench/perf/bigrat/mul | 267.1 | 237.6 | 0.889 |
| bench/perf/bigrat/quo | 68.6 | 65.1 | 0.950 |
| bench/perf/bigrat/cmp | 21.9 | 1.8 | 0.083 |
| bench/perf/bigrat/tofloat | 33.7 | 28.9 | 0.858 |
| bench/perf/bigrat/text | 18.2 | 17.4 | 0.958 |
| bench/perf/bigrat/parse | 51.4 | 42.4 | 0.826 |
