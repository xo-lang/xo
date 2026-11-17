kind: perf
module: mime
result: pass
commit: 57d0505e3
source: d0439fe7b222c44fb20a3d7d02b323ed27904633d8c939350b2be475999371d8
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" mime
measure: 7 cases of 1 twin directories, worst Xo / Go 0.959 over the cases above 5 ms (Proven at or under 1.05), 7s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/mime; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/mime/none (under 5 ms: startup) | 1.1 | 1.1 | 0.951 |
| bench/perf/mime/parse | 26.0 | 24.9 | 0.959 |
| bench/perf/mime/format | 48.9 | 29.7 | 0.606 |
| bench/perf/mime/ext (under 5 ms: startup) | 4.0 | 3.7 | 0.929 |
| bench/perf/mime/detect | 17.9 | 16.3 | 0.911 |
| bench/perf/mime/qpenc | 88.8 | 73.1 | 0.823 |
| bench/perf/mime/qpdec | 214.2 | 58.0 | 0.271 |
