kind: perf
module: jpeg
result: pass
commit: 58b0c68a4
source: 019450388a9c7ef8436c7951c465cf4117b5e1755aad4b2ecc608bb05d56b1d4
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" jpeg
measure: 5 cases of 1 twin directories, worst Xo / Go 1.027 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/jpeg; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/jpeg/decode | 93.8 | 92.3 | 0.984 |
| bench/perf/jpeg/rgba | 102.6 | 105.3 | 1.027 |
| bench/perf/jpeg/none (under 5 ms: startup) | 0.6 | 0.4 | 0.630 |
| bench/perf/jpeg/encode | 72.2 | 62.3 | 0.862 |
| bench/perf/jpeg/encrgba | 81.1 | 72.3 | 0.891 |
