kind: perf
module: otp
result: fail
commit: f539d8fc3
source: d1b9be80ca1045a69ece1383e77bc2bdd903852cc96e9591ea295f20bb6e65a8
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" otp
measure: 4 cases of 1 twin directories, worst Xo / Go 1.066 over the cases above 5 ms (Proven at or under 1.05), 9s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/otp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/otp/none (under 5 ms: startup) | 0.6 | 0.5 | 0.828 |
| bench/perf/otp/sha1 | 97.7 | 89.2 | 0.913 |
| bench/perf/otp/sha256 | 91.2 | 97.2 | 1.066 |
| bench/perf/otp/sha512 | 190.5 | 201.2 | 1.056 |
