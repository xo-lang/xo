kind: perf
module: go
result: fail
commit: ff00a8136
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" go
measure: 4 cases of 1 twin directories, worst Xo / Go 1.596 over the cases above 5 ms (Proven at or under 1.05), 15s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gobridge; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/none | 8.0 | 12.7 | 1.596 |
| bench/perf/gobridge/str | 32.8 | 33.3 | 1.014 |
| bench/perf/gobridge/bytes | 24.6 | 28.3 | 1.150 |
| bench/perf/gobridge/int | 130.4 | 137.3 | 1.053 |
