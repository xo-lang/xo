kind: perf
module: go
result: fail
commit: b45733fc8
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" go
measure: 4 cases of 1 twin directories, worst Xo / Go 1.892 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gobridge; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/none | 8.2 | 15.4 | 1.892 |
| bench/perf/gobridge/str | 31.3 | 32.8 | 1.048 |
| bench/perf/gobridge/bytes | 24.0 | 28.6 | 1.194 |
| bench/perf/gobridge/int | 134.2 | 145.3 | 1.083 |
