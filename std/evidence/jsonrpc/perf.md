kind: perf
module: jsonrpc
result: pass
commit: 502a0170c
source: ed7e81cea362f8a974a3fa120a14002295d05bb2bcce8fe65c745af359d730ce
date: 2026-11-17
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" jsonrpc
measure: 3 cases of 1 twin directories, worst Xo / Go 0.478 over the cases above 5 ms (Proven at or under 1.05), 9s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/jsonrpc; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/jsonrpc/none (under 5 ms: startup) | 0.7 | 0.5 | 0.674 |
| bench/perf/jsonrpc/encode | 61.4 | 29.3 | 0.478 |
| bench/perf/jsonrpc/decode | 622.5 | 52.3 | 0.084 |
