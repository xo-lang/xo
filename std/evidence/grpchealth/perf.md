kind: perf
module: grpchealth
result: pass
commit: b422ffc0d
source: a4525de8e1e865ac4ad852abbd3091e7f9e01ccda903114b32f1094ea36db2c6
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" grpchealth
measure: 3 cases of 1 twin directories, worst Xo / Go 0.930 over the cases above 5 ms (Proven at or under 1.05), 2m10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/grpchealth; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/grpchealth/none | 6.5 | 2.7 | 0.422 |
| bench/perf/grpchealth/check | 4341.3 | 4038.7 | 0.930 |
| bench/perf/grpchealth/list | 593.6 | 450.0 | 0.758 |
