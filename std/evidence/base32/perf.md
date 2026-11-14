kind: perf
module: base32
result: pass
commit: eab4a1a53
source: bfc2c5affdd311f6576402e0b3cf49e8ac20037dce3796aff1a97f321c9b65a6
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" base32
measure: 3 cases of 1 twin directories, worst Xo / Go 0.934 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/base32; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/base32/encode | 53.3 | 49.7 | 0.934 |
| bench/perf/base32/decode | 160.7 | 75.5 | 0.470 |
| bench/perf/base32/none | 10.5 | 4.9 | 0.465 |
