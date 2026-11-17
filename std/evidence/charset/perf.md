kind: perf
module: charset
result: pass
commit: 502a0170c
source: 0406c05bb9ad615c3799e31356da05ce77f23d292f7a1e605836330c588ed86c
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" charset
measure: 5 cases of 1 twin directories, worst Xo / Go 0.921 over the cases above 5 ms (Proven at or under 1.05), 4s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/charset; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/charset/none | 7.5 | 5.2 | 0.691 |
| bench/perf/charset/utf16_encode | 37.0 | 30.4 | 0.822 |
| bench/perf/charset/utf16_decode | 32.9 | 28.2 | 0.859 |
| bench/perf/charset/w1252_decode | 27.3 | 25.2 | 0.921 |
| bench/perf/charset/w1252_encode | 44.9 | 29.6 | 0.659 |
