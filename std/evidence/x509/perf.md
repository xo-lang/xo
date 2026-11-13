kind: perf
module: x509
result: pass
commit: 3f7c914b2
source: eef73db425ba8ed2fb9394257cf00b4e4a4a635fef903c0060aa87ec10819dd3
date: 2026-11-13
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" x509
measure: 7 cases of 1 twin directories, worst Xo / Go 0.940 over the cases above 5 ms (Proven at or under 1.05), 58s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/x509; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/x509/none (under 5 ms: startup) | 0.8 | 0.5 | 0.598 |
| bench/perf/x509/parse | 444.5 | 417.7 | 0.940 |
| bench/perf/x509/verify | 668.4 | 507.2 | 0.759 |
| bench/perf/x509/verify150 | 281.9 | 221.2 | 0.785 |
| bench/perf/x509/pool150 | 281.8 | 226.2 | 0.803 |
| bench/perf/x509/create | 1066.6 | 858.8 | 0.805 |
| bench/perf/x509/crl | 461.3 | 154.9 | 0.336 |
