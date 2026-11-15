kind: perf
module: crc64
result: pass
commit: eaa07eb3b
source: 3bd2152457fa866fea42d7ce226f5e0ed10c3d399dff43376e90ef16b1c1f199
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" crc64
measure: 4 cases of 1 twin directories, worst Xo / Go 1.019 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/crc64; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/crc64/none (under 5 ms: startup) | 0.8 | 0.7 | 0.823 |
| bench/perf/crc64/sum | 114.6 | 116.8 | 1.019 |
| bench/perf/crc64/keys | 45.5 | 21.2 | 0.465 |
| bench/perf/crc64/update | 59.1 | 59.2 | 1.003 |
