kind: perf
module: cli
result: fail
commit: 3f0e5e79d
source: 44e9258973b8c6c0b9148cb6406efe9e94a319d1c7ee344d5b3e7be786a73ca3
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" cli
measure: 2 cases of 1 twin directories, worst Xo / Go 1.718 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/cli; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/cli/none (under 5 ms: startup) | 0.6 | 0.5 | 0.770 |
| bench/perf/cli/parse | 134.5 | 231.1 | 1.718 |
