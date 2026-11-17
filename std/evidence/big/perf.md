kind: perf
module: big
result: pass
commit: 9e052eedd
source: 276f4d1d331a09cc6f16824f37afbf73047651917b08e6db6c18a17a6102b468
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" big
measure: 21 cases of 3 twin directories, worst Xo / Go 1.028 over the cases above 5 ms (Proven at or under 1.05), 18s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/big, bench/perf/bigfloat, bench/perf/bigrat; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/big/none (under 5 ms: startup) | 0.7 | 0.5 | 0.762 |
| bench/perf/big/mul | 8.5 | 8.7 | 1.024 |
| bench/perf/big/quorem | 9.4 | 9.7 | 1.028 |
| bench/perf/big/text (under 5 ms: startup) | 4.9 | 4.8 | 0.978 |
| bench/perf/big/parse | 7.1 | 3.3 | 0.470 |
| bench/perf/big/exp | 57.8 | 47.2 | 0.817 |
| bench/perf/bigfloat/none (under 5 ms: startup) | 0.6 | 0.5 | 0.891 |
| bench/perf/bigfloat/add | 11.8 | 12.1 | 1.023 |
| bench/perf/bigfloat/mul | 14.5 | 12.1 | 0.830 |
| bench/perf/bigfloat/quo | 22.7 | 19.8 | 0.871 |
| bench/perf/bigfloat/sqrt | 10.6 | 9.1 | 0.862 |
| bench/perf/bigfloat/text | 36.8 | 35.3 | 0.961 |
| bench/perf/bigfloat/parse | 20.2 | 14.1 | 0.697 |
| bench/perf/bigrat/none (under 5 ms: startup) | 0.7 | 0.5 | 0.796 |
| bench/perf/bigrat/add | 149.8 | 136.8 | 0.913 |
| bench/perf/bigrat/mul | 254.0 | 226.6 | 0.892 |
| bench/perf/bigrat/quo | 65.3 | 63.1 | 0.967 |
| bench/perf/bigrat/cmp | 21.1 | 1.8 | 0.086 |
| bench/perf/bigrat/tofloat | 33.0 | 27.9 | 0.845 |
| bench/perf/bigrat/text | 17.9 | 16.5 | 0.927 |
| bench/perf/bigrat/parse | 49.4 | 40.8 | 0.825 |
