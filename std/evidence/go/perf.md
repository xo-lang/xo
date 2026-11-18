kind: perf
module: go
result: fail
commit: 8b0b348a0
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" go
measure: 4 cases of 1 twin directories, worst Xo / Go 6.435 over the cases above 5 ms (Proven at or under 1.05), 19s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gobridge; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/none | 8.2 | 13.3 | 1.617 |
| bench/perf/gobridge/str | 34.1 | 156.4 | 4.586 |
| bench/perf/gobridge/bytes | 25.6 | 164.7 | 6.435 |
| bench/perf/gobridge/int | 136.4 | 247.0 | 1.811 |
