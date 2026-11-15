kind: perf
module: uuid
result: pass
commit: 1d09c6220
source: c3cacf57c4b2235e9fff8cf0544f04a5c6fba12342c82c322019488bc35be316
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" uuid
measure: 4 cases of 1 twin directories, worst Xo / Go 0.954 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/uuid; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/uuid/none (under 5 ms: startup) | 1.0 | 1.0 | 0.924 |
| bench/perf/uuid/parse | 11.9 | 11.3 | 0.954 |
| bench/perf/uuid/string | 8.2 | 6.0 | 0.729 |
| bench/perf/uuid/bytes (under 5 ms: startup) | 1.4 | 1.5 | 1.067 |
