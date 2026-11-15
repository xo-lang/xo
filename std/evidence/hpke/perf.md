kind: perf
module: hpke
result: pass
commit: 93473189e
source: 64842d194a05012a2cb7ee5298950ff4e97f855c3b65d5253f767e94b4fd0cea
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" hpke
measure: 3 cases of 1 twin directories, worst Xo / Go 0.817 over the cases above 5 ms (Proven at or under 1.05), 17s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/hpke; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/hpke/none (under 5 ms: startup) | 0.8 | 0.6 | 0.791 |
| bench/perf/hpke/setup | 771.8 | 630.8 | 0.817 |
| bench/perf/hpke/stream | 59.2 | 47.9 | 0.808 |
