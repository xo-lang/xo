# Decision 0500 (LLVM var homes): bench/perf/tlsloop --ab, 2026-11-04

xo-lang-4. linux/arm64 offload host 1, `xo bench --ab --rounds=3 --warmup=1`,
bench/perf/tlsloop built with `--backend=llvm` by each side's xo and std:
A = 9e2c09b13 (main, 0500's parent), B = 5f72b637d (0500). 20,000 POSTs on one
kept TLS 1.3 connection in one process. Script:
~/xo-lead/notes/xo-lang-4/scripts/abtls-remote.sh.

| workload | metric | A median | B median | B / A | 95% interval | verdict |
|---|---|---|---|---|---|---|
| small (16 B) | wall_ms | 1047 | 1014 | 0.969 | 0.969 to 0.989 | better |
| small (16 B) | cpu_ms | 1243 | 1182 | 0.951 | 0.947 to 0.989 | better |
| small (16 B) | max_rss_kb | 6720 | 6312 | 0.939 | 0.889 to 0.996 | better |
| big (16 KiB) | wall_ms | 1374 | 1371 | 0.998 | 0.962 to 1.0 | better |
| big (16 KiB) | cpu_ms | 1959 | 1956 | 0.998 | 0.96 to 1.006 | within noise |
| big (16 KiB) | max_rss_kb | 8128 | 8104 | 0.997 | 0.966 to 1.046 | within noise |

Binary: 6,628,720 to 6,543,304 bytes (-85,416). Both sides print the same
bytes received (small 320000, big 327680000). The small workload is the
per-record overhead (tlsio.put's copies of the 968 byte TLS 1.3 client); with
16 KiB records the AES-GCM work dominates.
