kind: perf
module: amqp
result: fail
commit: a2dcc9bc7
source: da3506e1627657283601c32abbb035fe52e01c7009fa6f310229515deeab3a57
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" amqp
measure: 3 cases of 1 twin directories, worst Xo / Go 1.160 over the cases above 5 ms (Proven at or under 1.05), 9s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/amqp; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/amqp/none (under 5 ms: startup) | 0.7 | 0.5 | 0.697 |
| bench/perf/amqp/encode | 199.9 | 231.8 | 1.160 |
| bench/perf/amqp/decode | 181.6 | 192.1 | 1.057 |
