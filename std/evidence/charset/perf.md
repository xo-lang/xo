kind: perf
module: charset
result: fail
commit: 117bb3891
source: 58d3a4d4a46cd1efb235a80901d376fe40f4681ad3c603023e0ffdf96a0e1854
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" charset
measure: 5 cases of 1 twin directories, worst Xo / Go 1.350 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/charset; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/charset/none | 7.6 | 5.2 | 0.676 |
| bench/perf/charset/utf16_encode | 37.9 | 31.0 | 0.819 |
| bench/perf/charset/utf16_decode | 33.2 | 41.2 | 1.240 |
| bench/perf/charset/w1252_decode | 26.8 | 36.1 | 1.350 |
| bench/perf/charset/w1252_encode | 44.4 | 30.0 | 0.676 |
