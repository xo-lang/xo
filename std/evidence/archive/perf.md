kind: perf
module: archive
result: fail
commit: 117bb3891
source: e2562fb280b3765d38ac033a49684594f4e0c533f00355f3e270e6c652003169
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" archive
measure: 8 cases of 1 twin directories, worst Xo / Go 1.192 over the cases above 5 ms (Proven at or under 1.05), 26s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/archive; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/archive/none (under 5 ms: startup) | 1.0 | 0.7 | 0.746 |
| bench/perf/archive/tar | 15.1 | 15.1 | 0.998 |
| bench/perf/archive/untar | 23.6 | 8.5 | 0.360 |
| bench/perf/archive/zip | 442.1 | 497.4 | 1.125 |
| bench/perf/archive/unzip | 82.5 | 72.0 | 0.873 |
| bench/perf/archive/zip_reader | 82.1 | 71.5 | 0.870 |
| bench/perf/archive/zip_writer | 445.7 | 531.4 | 1.192 |
| bench/perf/archive/zip_open | 90.6 | 77.6 | 0.856 |
