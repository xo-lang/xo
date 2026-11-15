kind: perf
module: xmlcodec
result: pass
commit: 7645bf7aa
source: 0a8ed0b8ab1f9804c74ba73b4a9fa3a311f8f02a40b4dec7540a3b95dc8167c2
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" xmlcodec
measure: 3 cases of 1 twin directories, worst Xo / Go 0.588 over the cases above 5 ms (Proven at or under 1.05), 13s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/xmlcodec; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/xmlcodec/none (under 5 ms: startup) | 0.7 | 0.5 | 0.775 |
| bench/perf/xmlcodec/encode | 174.2 | 83.2 | 0.478 |
| bench/perf/xmlcodec/decode | 617.8 | 363.5 | 0.588 |
