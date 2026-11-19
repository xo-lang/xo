kind: stress
module: task
result: pass
commit: 2deb4a4c2
source: ad80318929b903120251a1ccac0703c27d3cdff228b2c975974b31d113840193
date: 2026-11-19
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" task
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h0m9s

Stress evidence (decision 0616): the program std/evidence/task/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: task stress: 3004014776 checks by 64 tasks in 30 min, 0 mismatches, shared semaphore held by at most 8 of 8, rss 25 MB at the mark (median of minutes 15 to 17), 27 MB at the end (median of the last 3 minutes; 6.0%), peak 27 MB, pass, 30m0s
- arm64 run 2: task stress: 3055090710 checks by 64 tasks in 30 min, 0 mismatches, shared semaphore held by at most 7 of 8, rss 24 MB at the mark (median of minutes 15 to 17), 25 MB at the end (median of the last 3 minutes; 7.0%), peak 26 MB, pass, 30m0s
- llvm run 1: task stress: 2396315257 checks by 64 tasks in 30 min, 0 mismatches, shared semaphore held by at most 7 of 8, rss 23 MB at the mark (median of minutes 15 to 17), 26 MB at the end (median of the last 3 minutes; 11.0%), peak 26 MB, pass, 30m0s
- llvm run 2: task stress: 2364100893 checks by 64 tasks in 30 min, 0 mismatches, shared semaphore held by at most 8 of 8, rss 23 MB at the mark (median of minutes 15 to 17), 24 MB at the end (median of the last 3 minutes; 5.0%), peak 25 MB, pass, 30m0s
- arm64 --check=all: task stress: 198723567 checks by 64 tasks in 5 min, 0 mismatches, shared semaphore held by at most 7 of 8, rss not sampled, pass; xo leakcheck: 0 live of 1129065354 allocations, 5m0s
