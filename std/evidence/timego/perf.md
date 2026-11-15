kind: perf
module: timego
result: pass
commit: 117bb3891
source: 3e379f6ef5dc8b9448ea885076a004954a42061c731038d909e8360e01716aff
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" timego
measure: 3 cases of 1 twin directories, worst Xo / Go 0.966 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/timego; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/timego/none (under 5 ms: startup) | 0.6 | 0.5 | 0.900 |
| bench/perf/timego/format | 33.2 | 32.0 | 0.966 |
| bench/perf/timego/parse | 9.1 | 8.5 | 0.942 |
