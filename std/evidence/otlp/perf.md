kind: perf
module: otlp
result: pass
commit: f095943e6
source: 0ceb64256983fe748451d6d52396d060a476853003dc3d5589ae7337a2e94ded
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" otlp
measure: 3 cases of 1 twin directories, worst Xo / Go 0.459 over the cases above 5 ms (Proven at or under 1.05), 23s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/otlp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/otlp/none (under 5 ms: startup) | 3.1 | 1.3 | 0.410 |
| bench/perf/otlp/traces | 1167.4 | 260.6 | 0.223 |
| bench/perf/otlp/metrics | 435.5 | 199.9 | 0.459 |
