kind: perf
module: image
result: pass
commit: 5be17ca4d
source: c85c4d12fe1131778e9f0fb7727e178a1db7c4490367a18b6420530ccfae1bf5
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" image
measure: 5 cases of 1 twin directories, worst Xo / Go 0.960 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/image; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/image/over | 25.1 | 24.1 | 0.960 |
| bench/perf/image/src | 25.8 | 22.2 | 0.861 |
| bench/perf/image/gray | 18.6 | 13.8 | 0.741 |
| bench/perf/image/none | 13.8 | 10.1 | 0.735 |
| bench/perf/image/copy | 13.6 | 10.4 | 0.769 |
