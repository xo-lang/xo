kind: perf
module: cbor
result: pass
commit: ce0a8ffb1
source: 8ae746b2c4c5368bc015ad38a30024bdc9aca26c273a6f96749a2222e25b6b7c
date: 2026-11-16
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" cbor
measure: 6 cases of 1 twin directories, worst Xo / Go 0.955 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/cbor; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/cbor/none (under 5 ms: startup) | 0.9 | 0.6 | 0.677 |
| bench/perf/cbor/decode | 20.0 | 9.4 | 0.470 |
| bench/perf/cbor/encode | 15.1 | 4.0 | 0.266 |
| bench/perf/cbor/canon | 19.4 | 4.6 | 0.236 |
| bench/perf/cbor/udecode | 11.8 | 11.2 | 0.955 |
| bench/perf/cbor/umarshal (under 5 ms: startup) | 4.1 | 2.8 | 0.676 |
