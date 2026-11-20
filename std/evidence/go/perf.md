kind: perf
module: go
result: fail
commit: f345ced72
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-20
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 30 -host "linux/arm64 host 2" go
measure: 4 cases of 1 twin directories, worst Xo / Go 1.254 over the cases above 5 ms (Proven at or under 1.05), 14s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 30 over bench/perf/gobridge, the two programs alternating; Go ms and Xo ms are the medians of wall time, Xo / Go the median of the per-round ratios (decision 0726; a case under 10 ms of Go runs until each side has run 1 s).

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/none | 4.7 | 5.8 | 1.254 |
| bench/perf/gobridge/str | 26.6 | 30.8 | 1.152 |
| bench/perf/gobridge/bytes | 19.4 | 22.2 | 1.144 |
| bench/perf/gobridge/int | 113.1 | 107.3 | 0.948 |
