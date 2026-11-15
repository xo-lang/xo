kind: perf
module: cbor
result: fail
commit: 117bb3891
source: 106c5dd7e0ae0305f882bf7f843510e084eec1d387c159c52a7474cf96c1f6f4
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" cbor
measure: 6 cases of 1 twin directories, worst Xo / Go 1.171 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/cbor; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/cbor/none (under 5 ms: startup) | 0.9 | 0.7 | 0.749 |
| bench/perf/cbor/decode | 19.7 | 9.0 | 0.459 |
| bench/perf/cbor/encode | 14.9 | 3.9 | 0.260 |
| bench/perf/cbor/canon | 18.6 | 4.5 | 0.242 |
| bench/perf/cbor/udecode | 11.9 | 13.9 | 1.171 |
| bench/perf/cbor/umarshal (under 5 ms: startup) | 4.1 | 2.8 | 0.684 |
