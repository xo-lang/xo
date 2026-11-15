kind: perf
module: yaml
result: pass
commit: 117bb3891
source: 796c48db28dc858005b7084dd33af0297b375dfa429513acfe51721a9b7275ba
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" yaml
measure: 4 cases of 1 twin directories, worst Xo / Go 1.001 over the cases above 5 ms (Proven at or under 1.05), 33s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/yamlderive; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/yamlderive/parse | 648.5 | 649.0 | 1.001 |
| bench/perf/yamlderive/decode | 743.1 | 611.2 | 0.823 |
| bench/perf/yamlderive/encode | 285.6 | 227.3 | 0.796 |
| bench/perf/yamlderive/none (under 5 ms: startup) | 0.8 | 0.5 | 0.633 |
