kind: perf
module: big
result: pass
commit: 1b41742b2
source: 0d7e39ec5056c83ff43a1b7db517b3a37dcda51dbde0ea27f446f47f264aca73
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" big
measure: 21 cases of 3 twin directories, worst Xo / Go 1.013 over the cases above 5 ms (Proven at or under 1.05), 22s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/big, bench/perf/bigfloat, bench/perf/bigrat; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/big/none (under 5 ms: startup) | 1.7 | 1.2 | 0.733 |
| bench/perf/big/mul | 14.2 | 12.0 | 0.847 |
| bench/perf/big/quorem | 15.7 | 10.5 | 0.669 |
| bench/perf/big/text | 9.0 | 6.2 | 0.686 |
| bench/perf/big/parse | 7.8 | 4.0 | 0.521 |
| bench/perf/big/exp | 61.4 | 62.2 | 1.013 |
| bench/perf/bigfloat/none (under 5 ms: startup) | 1.6 | 1.2 | 0.787 |
| bench/perf/bigfloat/add | 18.7 | 14.4 | 0.771 |
| bench/perf/bigfloat/mul | 25.0 | 13.5 | 0.540 |
| bench/perf/bigfloat/quo | 30.7 | 19.6 | 0.640 |
| bench/perf/bigfloat/sqrt | 14.8 | 9.5 | 0.639 |
| bench/perf/bigfloat/text | 37.5 | 34.5 | 0.920 |
| bench/perf/bigfloat/parse | 25.2 | 13.0 | 0.515 |
| bench/perf/bigrat/none (under 5 ms: startup) | 1.5 | 1.1 | 0.718 |
| bench/perf/bigrat/add | 156.6 | 143.2 | 0.915 |
| bench/perf/bigrat/mul | 258.9 | 256.6 | 0.991 |
| bench/perf/bigrat/quo | 80.1 | 60.7 | 0.758 |
| bench/perf/bigrat/cmp | 28.0 | 2.5 | 0.091 |
| bench/perf/bigrat/tofloat | 49.8 | 27.7 | 0.556 |
| bench/perf/bigrat/text | 25.8 | 21.5 | 0.833 |
| bench/perf/bigrat/parse | 46.7 | 44.0 | 0.942 |
