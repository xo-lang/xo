kind: perf
module: toml
result: fail
commit: 57d0505e3
source: 6c2fe6dd7e1b30c5beee7405191f3a940fa5b33d06de028a41cc3da3dc5a7dab
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" toml
measure: 3 cases of 1 twin directories, worst Xo / Go 1.193 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/toml; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/toml/none (under 5 ms: startup) | 0.7 | 0.5 | 0.740 |
| bench/perf/toml/decode | 48.9 | 58.3 | 1.193 |
| bench/perf/toml/encode | 34.6 | 17.8 | 0.516 |
