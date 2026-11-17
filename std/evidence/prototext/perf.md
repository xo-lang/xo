kind: perf
module: prototext
result: pass
commit: 52003ef34
source: 6b9dd2969f19f2af7b853a97ab5ac7b189138ebb3ce1c82e18c3bbc2ad602863
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" prototext
measure: 3 cases of 1 twin directories, worst Xo / Go 1.048 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/prototext; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/prototext/none (under 5 ms: startup) | 0.9 | 0.6 | 0.652 |
| bench/perf/prototext/marshal | 41.5 | 40.7 | 0.979 |
| bench/perf/prototext/format | 43.1 | 45.2 | 1.048 |
