kind: perf
module: adler32
result: pass
commit: eaa07eb3b
source: 3404e8e15f502d2d00e20369198cc5818488606ad73071f781b925f39c579f9e
date: 2026-11-15
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" adler32
measure: 4 cases of 1 twin directories, worst Xo / Go 0.969 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/adler32; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/adler32/none (under 5 ms: startup) | 0.8 | 0.7 | 0.797 |
| bench/perf/adler32/sum | 68.9 | 66.8 | 0.969 |
| bench/perf/adler32/keys | 15.4 | 14.2 | 0.918 |
| bench/perf/adler32/update | 35.6 | 34.0 | 0.954 |
