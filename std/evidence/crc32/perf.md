kind: perf
module: crc32
result: pass
commit: 8264da41a
source: 15c9a19a15daad089c8b8979d222710f47020953b8c2ce70497aec1771a3f07b
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" crc32
measure: 6 cases of 1 twin directories, worst Xo / Go 0.978 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/crc32; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/crc32/none (under 5 ms: startup) | 0.9 | 0.7 | 0.785 |
| bench/perf/crc32/crc32 | 13.5 | 13.2 | 0.978 |
| bench/perf/crc32/crc32c | 13.4 | 5.1 | 0.381 |
| bench/perf/crc32/keys | 14.0 | 7.2 | 0.515 |
| bench/perf/crc32/keysc | 9.8 | 9.4 | 0.965 |
| bench/perf/crc32/update | 13.9 | 13.2 | 0.953 |
