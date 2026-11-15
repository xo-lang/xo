kind: perf
module: jwt
result: pass
commit: 374c25851
source: 2877c5964ce6740faf79a781cf3dbfa2b1420b5b149903a62f648b969e8f6a2e
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" jwt
measure: 6 cases of 1 twin directories, worst Xo / Go 0.935 over the cases above 5 ms (Proven at or under 1.05), 6s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/jwt; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/jwt/none (under 5 ms: startup) | 2.5 | 1.9 | 0.732 |
| bench/perf/jwt/hs256_sign | 24.2 | 14.6 | 0.603 |
| bench/perf/jwt/hs256_verify | 6.0 | 3.9 | 0.654 |
| bench/perf/jwt/rs256_verify | 49.9 | 46.6 | 0.935 |
| bench/perf/jwt/es256_verify | 78.1 | 69.2 | 0.886 |
| bench/perf/jwt/eddsa_verify | 62.9 | 48.5 | 0.772 |
