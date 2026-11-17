kind: perf
module: httpproxy
result: fail
commit: a2dcc9bc7
source: 92b250b062c1c3d0d169fde29c1f3a5cf8e5031aaae128817168ab0c187ec0d5
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" httpproxy
measure: 3 cases of 1 twin directories, worst Xo / Go 1.665 over the cases above 5 ms (Proven at or under 1.05), 18s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/httpproxy_rules; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/httpproxy_rules/none (under 5 ms: startup) | 0.7 | 0.5 | 0.741 |
| bench/perf/httpproxy_rules/route | 304.6 | 507.2 | 1.665 |
| bench/perf/httpproxy_rules/rules | 446.3 | 381.3 | 0.854 |
