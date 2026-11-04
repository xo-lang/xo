# std/multipart and std/udp against Go (2026-11-04)

Twins bench/perf/multipart and bench/perf/udp (checksums equal, twincheck),
linux/arm64 host 1, 7 rounds (n=6); times Go first, ratio Xo over Go.

| workload | Go | Xo | Xo / Go | range |
|---|---|---|---|---|
| multipart parse (16 parts, 40 KB, 4,000 times), before | 229.3 | 320.9 | 1.399 | 1.352-1.455 |
| multipart parse, after index_of through Bytes.find | 220.8 | 49.1 | 0.222 | 0.215-0.224 |
| multipart write | 116.4 | 28.27 | 0.243 | 0.237-0.249 |
| udp echo (60,000 loopback datagrams) | 169.3 | 164.0 | 0.969 | 0.951-0.978 |

Cause of the multipart gap: scan's index_of looked for the boundary with a
byte loop that called has_prefix at every offset (read_part held 84% of the
profile); it now uses Bytes.find.
