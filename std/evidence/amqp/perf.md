kind: perf
module: amqp
result: pass
commit: 09f097a2d
source: a3a65a2872cbced5ec24ff6b89126820556d596707d9c1baaf125770cc0024d1
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" amqp
measure: 3 cases of 1 twin directories, worst Xo / Go 0.728 over the cases above 5 ms (Proven at or under 1.05), 12s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/amqp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/amqp/none (under 5 ms: startup) | 2.2 | 1.2 | 0.561 |
| bench/perf/amqp/encode | 315.3 | 206.7 | 0.656 |
| bench/perf/amqp/decode | 246.3 | 179.4 | 0.728 |
