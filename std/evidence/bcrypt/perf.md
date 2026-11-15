kind: perf
module: bcrypt
result: pass
commit: 117bb3891
source: 48dd32f4a191b64dde3fae6a69213beaa89d014a233900c6dd7c3c44d08f0938
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" bcrypt
measure: 3 cases of 1 twin directories, worst Xo / Go 1.000 over the cases above 5 ms (Proven at or under 1.05), 36s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/bcrypt; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/bcrypt/none (under 5 ms: startup) | 0.6 | 0.5 | 0.813 |
| bench/perf/bcrypt/hash | 882.2 | 882.1 | 1.000 |
| bench/perf/bcrypt/compare | 882.0 | 880.8 | 0.999 |
