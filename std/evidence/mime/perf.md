kind: perf
module: mime
result: fail
commit: eaa07eb3b
source: 4f8511b0f318fcecd9d8f769d1bfc656fdfa08d6230f2eadd9e0350ea28bdf4a
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" mime
measure: 5 cases of 1 twin directories, worst Xo / Go 1.179 over the cases above 5 ms (Proven at or under 1.05), 3s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/mime; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/mime/none (under 5 ms: startup) | 1.1 | 1.0 | 0.899 |
| bench/perf/mime/parse | 25.7 | 30.3 | 1.179 |
| bench/perf/mime/format | 48.3 | 27.7 | 0.574 |
| bench/perf/mime/ext (under 5 ms: startup) | 4.0 | 3.4 | 0.868 |
| bench/perf/mime/detect | 17.6 | 15.4 | 0.874 |
