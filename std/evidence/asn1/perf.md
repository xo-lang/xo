kind: perf
module: asn1
result: pass
commit: 554dc5eff
source: 201b8ea822f0c2ece366c8fa97cd95e2c3739144143f7d54fe2373a394804790
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" asn1
measure: 3 cases of 1 twin directories, worst Xo / Go 0.431 over the cases above 5 ms (Proven at or under 1.05), 11s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/asn1; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/asn1/none (under 5 ms: startup) | 0.7 | 0.6 | 0.783 |
| bench/perf/asn1/parse | 322.6 | 139.2 | 0.431 |
| bench/perf/asn1/der | 424.1 | 146.9 | 0.346 |
