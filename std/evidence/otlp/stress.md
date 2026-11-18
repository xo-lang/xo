kind: stress
module: otlp
result: pass
commit: ccafbe5be
source: f1840c01ae84c56ca373a7efc6e2619e88687c1561172f3e0075b4cb2c2711a1
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" otlp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h2m10s

Stress evidence (decision 0616): the program std/evidence/otlp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: otlp stress: 59823152 exports by 64 tasks in 30 min (57222144 traces and 5982291 metrics requests taken), 0 wrong, rss 46 MB at the mark (median of minutes 15 to 17), 46 MB at the end (median of the last 3 minutes; -0.0%), peak 51 MB, pass, 30m0s
- arm64 run 2: otlp stress: 64505818 exports by 64 tasks in 30 min (61701221 traces and 6450555 metrics requests taken), 0 wrong, rss 44 MB at the mark (median of minutes 15 to 17), 47 MB at the end (median of the last 3 minutes; 8.0%), peak 51 MB, pass, 30m0s
- llvm run 1: otlp stress: 54252916 exports by 64 tasks in 30 min (51894094 traces and 5425263 metrics requests taken), 0 wrong, rss 44 MB at the mark (median of minutes 15 to 17), 46 MB at the end (median of the last 3 minutes; 6.0%), peak 49 MB, pass, 30m0s
- llvm run 2: otlp stress: 59601558 exports by 64 tasks in 30 min (57010183 traces and 5960131 metrics requests taken), 0 wrong, rss 44 MB at the mark (median of minutes 15 to 17), 47 MB at the end (median of the last 3 minutes; 7.0%), peak 53 MB, pass, 30m30s
- arm64 --check=all: otlp stress: 3657208 exports by 64 tasks in 5 min (3498203 traces and 365693 metrics requests taken), 0 wrong, rss not sampled, pass; xo leakcheck: 0 live of 2364993673 allocations, 5m0s
