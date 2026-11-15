kind: perf
module: toml
result: fail
commit: 117bb3891
source: 3b3776a914248bb8569a47accda6d7827a92010c877c9dd3c50dab46cdcc4d86
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" toml
measure: 3 cases of 1 twin directories, worst Xo / Go 1.165 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/toml; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/toml/none (under 5 ms: startup) | 0.7 | 0.5 | 0.750 |
| bench/perf/toml/decode | 47.4 | 55.3 | 1.165 |
| bench/perf/toml/encode | 35.6 | 17.0 | 0.478 |
