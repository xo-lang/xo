kind: perf
module: amqp
result: fail
commit: b232ddf22
source: 18f8df3fd49ea711a5333bace51c680b26a551e3d62a252f388bea7fef3753bd
date: 2026-11-18
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" amqp
measure: 3 cases of 1 twin directories, worst Xo / Go 1.108 over the cases above 5 ms (Proven at or under 1.05), 10s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/amqp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/amqp/none (under 5 ms: startup) | 0.7 | 0.5 | 0.722 |
| bench/perf/amqp/encode | 209.5 | 232.2 | 1.108 |
| bench/perf/amqp/decode | 185.9 | 194.1 | 1.044 |
