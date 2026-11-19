kind: perf
module: tls
result: fail
commit: 54a75d591
source: 19a3ed1a7d5d95bf04cfaa4bc7b38ff540afc2093b284d6106902ae12c34e9a9
date: 2026-11-19
host: linux/amd64 host
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/amd64 host" tls
measure: 2 cases of 1 twin directories, worst Xo / Go 1.191 over the cases above 5 ms (Proven at or under 1.05), 3m14s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/tlsloop; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/tlsloop/small | 3576.4 | 4261.2 | 1.191 |
| bench/perf/tlsloop/big | 5426.2 | 5568.5 | 1.026 |
