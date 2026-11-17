kind: perf
module: archive
result: pass
commit: b4c777cf4
source: 41adc5a1b873fc4902ef0c232e9954ab76f0ad29f2825b588baf6769f4e3b3a5
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" archive
measure: 8 cases of 1 twin directories, worst Xo / Go 1.048 over the cases above 5 ms (Proven at or under 1.05), 25s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/archive; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/archive/none (under 5 ms: startup) | 1.1 | 0.8 | 0.704 |
| bench/perf/archive/tar | 15.1 | 15.9 | 1.048 |
| bench/perf/archive/untar | 23.7 | 7.0 | 0.296 |
| bench/perf/archive/zip | 465.0 | 439.8 | 0.946 |
| bench/perf/archive/unzip | 84.5 | 74.8 | 0.884 |
| bench/perf/archive/zip_reader | 84.4 | 73.7 | 0.874 |
| bench/perf/archive/zip_writer | 462.2 | 440.4 | 0.953 |
| bench/perf/archive/zip_open | 92.5 | 80.0 | 0.864 |
