kind: perf
module: term
result: fail
commit: 69007a3cb
source: b2f0878794d79e2590af1b88f27549b65878f9ef9b2b2eb83cb76e9478347a46
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" term
measure: 7 cases of 2 twin directories, worst Xo / Go 1.123 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/term, bench/perf/termline; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/term/none (under 5 ms: startup) | 0.6 | 0.5 | 0.753 |
| bench/perf/term/tty | 41.5 | 35.2 | 0.847 |
| bench/perf/term/paint | 20.8 | 19.7 | 0.947 |
| bench/perf/term/strip | 14.0 | 15.7 | 1.123 |
| bench/perf/termline/none (under 5 ms: startup) | 1.3 | 1.0 | 0.786 |
| bench/perf/termline/type | 32.3 | 32.8 | 1.014 |
| bench/perf/termline/wrap | 33.4 | 33.9 | 1.017 |
