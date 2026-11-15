kind: perf
module: gif
result: fail
commit: 117bb3891
source: f9a72eeced17838613dd362c1e576368792e010f421de37ba5563c561824ea60
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" gif
measure: 3 cases of 1 twin directories, worst Xo / Go 1.242 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/gif; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gif/encode | 25.8 | 32.1 | 1.242 |
| bench/perf/gif/decode | 29.7 | 28.5 | 0.961 |
| bench/perf/gif/none (under 5 ms: startup) | 2.0 | 1.9 | 0.973 |
