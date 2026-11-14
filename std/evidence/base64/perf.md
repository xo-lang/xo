kind: perf
module: base64
result: pass
commit: eab4a1a53
source: efd9b1731ce03a28532b686126bc1f938006e447a93cc3c6b91f8e9cdee37346
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" base64
measure: 3 cases of 1 twin directories, worst Xo / Go 0.865 over the cases above 5 ms (Proven at or under 1.05), 2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/base64; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/base64/encode | 49.6 | 26.8 | 0.542 |
| bench/perf/base64/decode | 35.4 | 30.6 | 0.865 |
| bench/perf/base64/none | 10.0 | 4.8 | 0.478 |
