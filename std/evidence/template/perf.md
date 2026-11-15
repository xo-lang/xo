kind: perf
module: template
result: pass
commit: 7645bf7aa
source: b80ca3fbad0dd21f6f7d0913815e4947aef80cb16a05ee179fbabf2346a1d7eb
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" template
measure: 3 cases of 1 twin directories, worst Xo / Go 0.607 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/template; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/template/none (under 5 ms: startup) | 0.8 | 0.6 | 0.740 |
| bench/perf/template/parse | 110.7 | 67.2 | 0.607 |
| bench/perf/template/render | 247.8 | 144.6 | 0.583 |
