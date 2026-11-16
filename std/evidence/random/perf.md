kind: perf
module: random
result: fail
commit: 536b01c2b
source: c8b5997a33ddca6a4701b27d4282fb03f4f3f6fcebc9bc0f351443a647cc2a2b
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" random
measure: 5 cases of 1 twin directories, worst Xo / Go 2.081 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/random; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/random/none (under 5 ms: startup) | 0.7 | 0.5 | 0.763 |
| bench/perf/random/normal | 9.5 | 17.4 | 1.827 |
| bench/perf/random/exp | 7.2 | 14.9 | 2.081 |
| bench/perf/random/perm | 14.2 | 25.8 | 1.816 |
| bench/perf/random/zipf | 22.9 | 42.7 | 1.867 |
