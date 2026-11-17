kind: perf
module: archive
result: pass
commit: 42eb91ace
source: 3f24b6af9b4c9eea91413ada780bcfa4a867ba1d1d386735136164f2a8b83a84
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" archive
measure: 8 cases of 1 twin directories, worst Xo / Go 0.958 over the cases above 5 ms (Proven at or under 1.05), 25s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/archive; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/archive/none (under 5 ms: startup) | 1.0 | 0.8 | 0.782 |
| bench/perf/archive/tar | 14.9 | 7.9 | 0.533 |
| bench/perf/archive/untar | 23.4 | 6.8 | 0.290 |
| bench/perf/archive/zip | 464.3 | 440.0 | 0.948 |
| bench/perf/archive/unzip | 84.8 | 75.0 | 0.885 |
| bench/perf/archive/zip_reader | 84.8 | 73.9 | 0.872 |
| bench/perf/archive/zip_writer | 460.8 | 441.4 | 0.958 |
| bench/perf/archive/zip_open | 93.0 | 79.6 | 0.856 |
