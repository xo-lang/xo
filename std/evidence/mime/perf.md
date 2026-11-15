kind: perf
module: mime
result: fail
commit: 117bb3891
source: 4f8511b0f318fcecd9d8f769d1bfc656fdfa08d6230f2eadd9e0350ea28bdf4a
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" mime
measure: 5 cases of 1 twin directories, worst Xo / Go 1.272 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/mime; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/mime/none (under 5 ms: startup) | 1.0 | 1.0 | 0.944 |
| bench/perf/mime/parse | 24.8 | 31.6 | 1.272 |
| bench/perf/mime/format | 47.1 | 27.4 | 0.581 |
| bench/perf/mime/ext (under 5 ms: startup) | 3.8 | 3.4 | 0.893 |
| bench/perf/mime/detect | 17.7 | 15.0 | 0.852 |
