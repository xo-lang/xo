kind: perf
module: otp
result: pass
commit: a64db4573
source: cddf012aa750d949ae49316091a5734b617f8241233091ef236da0826be15e2f
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" otp
measure: 4 cases of 1 twin directories, worst Xo / Go 0.989 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/otp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/otp/none (under 5 ms: startup) | 0.7 | 0.5 | 0.770 |
| bench/perf/otp/sha1 | 94.6 | 88.4 | 0.934 |
| bench/perf/otp/sha256 | 88.0 | 87.0 | 0.989 |
| bench/perf/otp/sha512 | 185.0 | 170.9 | 0.924 |
