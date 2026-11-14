kind: perf
module: fnv
result: pass
commit: 4357973f3
source: 5f34cd0ac2005b37cf77ebf324b2f8d539b5827efe52c87aebbb39fc17218cba
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" fnv
measure: 7 cases of 1 twin directories, worst Xo / Go 0.942 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/fnv; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/fnv/none (under 5 ms: startup) | 0.6 | 0.4 | 0.698 |
| bench/perf/fnv/fnv32 | 20.5 | 19.3 | 0.942 |
| bench/perf/fnv/fnv32a | 21.2 | 19.7 | 0.925 |
| bench/perf/fnv/fnv64 | 21.4 | 19.9 | 0.928 |
| bench/perf/fnv/fnv64a | 21.9 | 20.4 | 0.929 |
| bench/perf/fnv/fnv128 | 120.1 | 106.9 | 0.890 |
| bench/perf/fnv/fnv128a | 140.1 | 104.5 | 0.746 |
