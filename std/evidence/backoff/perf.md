kind: perf
module: backoff
result: pass
commit: 502a0170c
source: fa9d2b82f71c1a3d158fa8a54f2d0ae97260e5beb2ad91f562805f28db33037c
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" backoff
measure: 3 cases of 1 twin directories, worst Xo / Go 0.533 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/backoff; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/backoff/none (under 5 ms: startup) | 0.7 | 0.5 | 0.768 |
| bench/perf/backoff/schedule | 48.9 | 23.7 | 0.484 |
| bench/perf/backoff/jitter | 50.5 | 26.9 | 0.533 |
