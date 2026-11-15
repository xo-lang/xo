kind: perf
module: otp
result: fail
commit: 117bb3891
source: 0b00ee885ff6b1b7cd7715584be6a04a567fff4713f78055ce5133f87eaf913e
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" otp
measure: 4 cases of 1 twin directories, worst Xo / Go 1.115 over the cases above 5 ms (Proven at or under 1.05), 8s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/otp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/otp/none (under 5 ms: startup) | 0.6 | 0.5 | 0.897 |
| bench/perf/otp/sha1 | 91.8 | 102.3 | 1.115 |
| bench/perf/otp/sha256 | 86.4 | 93.8 | 1.086 |
| bench/perf/otp/sha512 | 179.3 | 182.7 | 1.019 |
