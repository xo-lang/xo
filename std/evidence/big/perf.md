kind: perf
module: big
result: fail
commit: b232ddf22
source: 276f4d1d331a09cc6f16824f37afbf73047651917b08e6db6c18a17a6102b468
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" big
measure: 21 cases of 3 twin directories, worst Xo / Go 1.119 over the cases above 5 ms (Proven at or under 1.05), 18s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/big, bench/perf/bigfloat, bench/perf/bigrat; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/big/none (under 5 ms: startup) | 0.7 | 0.6 | 0.852 |
| bench/perf/big/mul | 8.9 | 8.9 | 1.011 |
| bench/perf/big/quorem | 9.6 | 10.1 | 1.049 |
| bench/perf/big/text | 5.2 | 5.1 | 0.990 |
| bench/perf/big/parse | 7.5 | 3.4 | 0.453 |
| bench/perf/big/exp | 58.0 | 47.1 | 0.812 |
| bench/perf/bigfloat/none (under 5 ms: startup) | 0.7 | 0.6 | 0.879 |
| bench/perf/bigfloat/add | 11.8 | 13.2 | 1.119 |
| bench/perf/bigfloat/mul | 14.6 | 12.6 | 0.866 |
| bench/perf/bigfloat/quo | 23.6 | 21.1 | 0.897 |
| bench/perf/bigfloat/sqrt | 11.1 | 9.5 | 0.857 |
| bench/perf/bigfloat/text | 39.0 | 37.6 | 0.964 |
| bench/perf/bigfloat/parse | 21.2 | 14.7 | 0.695 |
| bench/perf/bigrat/none (under 5 ms: startup) | 0.6 | 0.5 | 0.792 |
| bench/perf/bigrat/add | 157.0 | 147.2 | 0.938 |
| bench/perf/bigrat/mul | 268.1 | 241.5 | 0.901 |
| bench/perf/bigrat/quo | 68.4 | 64.9 | 0.949 |
| bench/perf/bigrat/cmp | 21.5 | 1.8 | 0.085 |
| bench/perf/bigrat/tofloat | 34.2 | 29.0 | 0.850 |
| bench/perf/bigrat/text | 18.6 | 17.6 | 0.943 |
| bench/perf/bigrat/parse | 52.5 | 43.7 | 0.832 |
