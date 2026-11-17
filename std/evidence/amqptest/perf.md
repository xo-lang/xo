kind: perf
module: amqptest
result: pass
commit: a2dcc9bc7
source: cf1452d96b9fe806cc5d7ed121f7faf749f51880f8fba2e3a42d04c84147099e
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" amqptest
measure: 3 cases of 1 twin directories, worst Xo / Go 1.020 over the cases above 5 ms (Proven at or under 1.05), 5s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/amqptest; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/amqptest/none (under 5 ms: startup) | 2.7 | 1.0 | 0.376 |
| bench/perf/amqptest/pipe | 86.1 | 41.0 | 0.476 |
| bench/perf/amqptest/pubsub | 67.3 | 68.7 | 1.020 |
