kind: perf
module: gif
result: pass
commit: 5c19edc88
source: 215a7b62f899bead7b3204eeae406fdcb8eeb60adbb8736bce24382341e010c4
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" gif
measure: 3 cases of 1 twin directories, worst Xo / Go 0.909 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gif; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gif/encode | 26.7 | 24.3 | 0.909 |
| bench/perf/gif/decode | 29.6 | 26.5 | 0.895 |
| bench/perf/gif/none (under 5 ms: startup) | 1.9 | 2.0 | 1.068 |
