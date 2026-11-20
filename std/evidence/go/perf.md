kind: perf
module: go
result: fail
commit: 04eaa39a1
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-20
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" go
measure: 4 cases of 1 twin directories, worst Xo / Go 1.483 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gobridge; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/none | 5.4 | 8.0 | 1.483 |
| bench/perf/gobridge/str | 26.6 | 29.4 | 1.105 |
| bench/perf/gobridge/bytes | 19.5 | 22.3 | 1.142 |
| bench/perf/gobridge/int | 111.2 | 106.8 | 0.961 |
