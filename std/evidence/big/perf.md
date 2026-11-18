kind: perf
module: big
result: fail
commit: 09f097a2d
source: 276f4d1d331a09cc6f16824f37afbf73047651917b08e6db6c18a17a6102b468
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" big
measure: 21 cases of 3 twin directories, worst Xo / Go 1.174 over the cases above 5 ms (Proven at or under 1.05), 21s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/big, bench/perf/bigfloat, bench/perf/bigrat; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/big/none (under 5 ms: startup) | 1.6 | 1.1 | 0.706 |
| bench/perf/big/mul | 14.4 | 12.4 | 0.856 |
| bench/perf/big/quorem | 15.8 | 10.6 | 0.673 |
| bench/perf/big/text | 9.4 | 7.1 | 0.759 |
| bench/perf/big/parse | 8.2 | 4.1 | 0.502 |
| bench/perf/big/exp | 63.5 | 74.5 | 1.174 |
| bench/perf/bigfloat/none (under 5 ms: startup) | 1.6 | 1.1 | 0.699 |
| bench/perf/bigfloat/add | 19.2 | 15.2 | 0.795 |
| bench/perf/bigfloat/mul | 24.3 | 15.2 | 0.625 |
| bench/perf/bigfloat/quo | 28.9 | 18.9 | 0.655 |
| bench/perf/bigfloat/sqrt | 14.8 | 9.7 | 0.657 |
| bench/perf/bigfloat/text | 37.9 | 34.1 | 0.899 |
| bench/perf/bigfloat/parse | 26.0 | 13.7 | 0.526 |
| bench/perf/bigrat/none (under 5 ms: startup) | 1.5 | 1.1 | 0.732 |
| bench/perf/bigrat/add | 157.6 | 156.5 | 0.993 |
| bench/perf/bigrat/mul | 250.5 | 279.5 | 1.115 |
| bench/perf/bigrat/quo | 80.1 | 64.2 | 0.801 |
| bench/perf/bigrat/cmp | 28.3 | 2.5 | 0.089 |
| bench/perf/bigrat/tofloat | 48.2 | 29.1 | 0.604 |
| bench/perf/bigrat/text | 26.6 | 21.3 | 0.802 |
| bench/perf/bigrat/parse | 46.3 | 42.8 | 0.925 |
