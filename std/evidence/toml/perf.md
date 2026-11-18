kind: perf
module: toml
result: pass
commit: 09f097a2d
source: 6c2fe6dd7e1b30c5beee7405191f3a940fa5b33d06de028a41cc3da3dc5a7dab
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" toml
measure: 3 cases of 1 twin directories, worst Xo / Go 0.952 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/toml; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/toml/none (under 5 ms: startup) | 1.5 | 1.1 | 0.730 |
| bench/perf/toml/decode | 70.6 | 67.2 | 0.952 |
| bench/perf/toml/encode | 41.3 | 18.2 | 0.440 |
