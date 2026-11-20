kind: perf
module: go
result: pass
commit: ca1a819ca
source: 20ba3b9d9243d8ac8566f8ec0af9a8291d6eacd35b73ddc158a88cc18b5e7b66
date: 2026-11-20
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 30 -host "linux/arm64 host 2" go
measure: 8 cases of 1 twin directories, worst Xo / Go 1.018 over the cases above 5 ms (Proven at or under 1.05), 54s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 30 over bench/perf/gobridge, the two programs alternating; Go ms and Xo ms are the medians of wall time, Xo / Go the median of the per-round ratios (decision 0726; a case under 10 ms of Go runs until each side has run 1 s).

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/none | 101.7 | 102.7 | 1.011 |
| bench/perf/gobridge/str | 119.8 | 122.0 | 1.018 |
| bench/perf/gobridge/bytes | 82.0 | 82.5 | 1.006 |
| bench/perf/gobridge/int | 111.1 | 106.8 | 0.961 |
| bench/perf/gobridge/diag_startup (diagnostic, not judged) | 0.4 | 0.7 | 1.662 |
| bench/perf/gobridge/diag_none_checked (diagnostic, not judged) | 93.8 | 103.6 | 1.102 |
| bench/perf/gobridge/diag_str_checked (diagnostic, not judged) | 128.8 | 138.3 | 1.070 |
| bench/perf/gobridge/diag_bytes_checked (diagnostic, not judged) | 91.9 | 104.2 | 1.135 |
