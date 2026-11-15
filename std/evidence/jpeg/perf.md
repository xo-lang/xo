kind: perf
module: jpeg
result: pass
commit: f523cf523
source: 5f0d82e205789ddcdafa775bcd5797960a6fca94b5a193128bcba9c40c8e83e2
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" jpeg
measure: 5 cases of 1 twin directories, worst Xo / Go 1.013 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/jpeg; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/jpeg/decode | 94.7 | 94.0 | 0.993 |
| bench/perf/jpeg/rgba | 105.2 | 106.5 | 1.013 |
| bench/perf/jpeg/none (under 5 ms: startup) | 0.5 | 0.5 | 0.865 |
| bench/perf/jpeg/encode | 71.9 | 60.7 | 0.844 |
| bench/perf/jpeg/encrgba | 83.4 | 73.5 | 0.882 |
