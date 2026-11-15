kind: perf
module: protobuf
result: pass
commit: 117bb3891
source: 8383afbca00d2f52be73aac9e66e48ae93e7b7763954ed96b8f8e499b940cb54
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" protobuf
measure: 3 cases of 1 twin directories, worst Xo / Go 0.567 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/protobuf; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/protobuf/none (under 5 ms: startup) | 0.7 | 0.5 | 0.682 |
| bench/perf/protobuf/unmarshal | 40.7 | 23.0 | 0.567 |
| bench/perf/protobuf/marshal | 19.7 | 11.0 | 0.561 |
