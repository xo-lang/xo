kind: perf
module: jwt
result: fail
commit: 117bb3891
source: 3c3881747158cf6930fc8486b7c2d75a2c420b99352bd40bc49598bef588427a
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" jwt
measure: 6 cases of 1 twin directories, worst Xo / Go 1.692 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/jwt; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/jwt/none (under 5 ms: startup) | 2.5 | 1.7 | 0.696 |
| bench/perf/jwt/hs256_sign | 23.2 | 14.1 | 0.611 |
| bench/perf/jwt/hs256_verify | 5.7 | 3.7 | 0.653 |
| bench/perf/jwt/rs256_verify | 49.8 | 46.4 | 0.933 |
| bench/perf/jwt/es256_verify | 75.8 | 128.2 | 1.692 |
| bench/perf/jwt/eddsa_verify | 62.8 | 47.0 | 0.749 |
