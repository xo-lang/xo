kind: perf
module: http
result: pass
commit: 7ef8b3619
source: 47c495075e1c2834ec7e6004feaf1c9cbc8ccc841c9b223164ee16316fba68e3
date: 2026-11-13
host: linux/arm64 host 1
command: go run ./scripts/stdproven serverperf -reps 5 -host "linux/arm64 host 1" http
measure: 8 workload and backend figures, worst Xo / Go cost 0.943 (the worse of throughput and CPU or wall per figure; Proven at or under 1.05), 4m41s

Server performance against Go (decision 0616, Servers): bench/perf/http/proven.sh against Go twins on the native backends. A figure is the worst of its judged cost ratios (Go's rps over Xo's; Xo's CPU per request or wall over Go's); p99 is shown, not judged.

Figures: crud arm64 0.661, crud llvm 0.670, proxy arm64 0.748, proxy llvm 0.811, stream-stream arm64 0.943, stream-stream llvm 0.937, stream-send arm64 0.545, stream-send llvm 0.544

| workload | backend | metric | Go | Xo | cost Xo / Go |
|---|---|---|---|---|---|
| crud | arm64 | rps | 127228 | 192560 | 0.661 |
| crud | arm64 | cpu_us | 26.51 | 11.63 | 0.439 |
| crud | arm64 | p99_us (shown) | 1134 | 837 | 0.738 |
| crud | llvm | rps | 127228 | 189790 | 0.670 |
| crud | llvm | cpu_us | 26.51 | 11.95 | 0.451 |
| crud | llvm | p99_us (shown) | 1134 | 837 | 0.738 |
| proxy | arm64 | wall_s | 0.214843 | 0.160789 | 0.748 |
| proxy | llvm | wall_s | 0.214843 | 0.174174 | 0.811 |
| stream-stream | arm64 | wall_s | 0.155445 | 0.146542 | 0.943 |
| stream-stream | llvm | wall_s | 0.155445 | 0.145716 | 0.937 |
| stream-send | arm64 | wall_s | 0.152261 | 0.083002 | 0.545 |
| stream-send | llvm | wall_s | 0.152261 | 0.082835 | 0.544 |
