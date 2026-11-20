kind: perf
module: go
result: fail
commit: 470afc591
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-20
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 2" go
measure: 4 cases of 1 twin directories, worst Xo / Go 1.438 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gobridge; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/none | 5.5 | 7.8 | 1.438 |
| bench/perf/gobridge/str | 26.1 | 28.0 | 1.071 |
| bench/perf/gobridge/bytes | 19.1 | 25.3 | 1.322 |
| bench/perf/gobridge/int | 114.9 | 110.2 | 0.959 |
