kind: perf
module: htmlparse
result: pass
commit: ce0a8ffb1
source: b8c4511da24eb05beee0e09f7c47ea40a0fccdec2ad4263424bc86ed8df62fe1
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" htmlparse
measure: 5 cases of 1 twin directories, worst Xo / Go 0.936 over the cases above 5 ms (Proven at or under 1.05), 16s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/htmlparse; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/htmlparse/none (under 5 ms: startup) | 0.7 | 0.5 | 0.718 |
| bench/perf/htmlparse/parse | 247.4 | 189.4 | 0.766 |
| bench/perf/htmlparse/tok | 141.3 | 90.6 | 0.641 |
| bench/perf/htmlparse/ent | 381.1 | 348.3 | 0.914 |
| bench/perf/htmlparse/render | 53.0 | 49.6 | 0.936 |
