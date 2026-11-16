kind: perf
module: xml
result: pass
commit: 614956711
source: eabed39f6a91c9b36cec510fac8c0c63dd45c67e8fefa9bed6b80a8a2f872714
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" xml
measure: 5 cases of 1 twin directories, worst Xo / Go 0.741 over the cases above 5 ms (Proven at or under 1.05), 16s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/xml; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/xml/none (under 5 ms: startup) | 3.0 | 1.5 | 0.477 |
| bench/perf/xml/tokens | 218.4 | 118.0 | 0.540 |
| bench/perf/xml/stream | 219.7 | 116.1 | 0.529 |
| bench/perf/xml/source | 221.0 | 163.8 | 0.741 |
| bench/perf/xml/tree | 307.0 | 144.5 | 0.471 |
