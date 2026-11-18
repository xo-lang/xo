kind: stress
module: task
result: pass
commit: ca546cdd5
source: ad80318929b903120251a1ccac0703c27d3cdff228b2c975974b31d113840193
date: 2026-11-18
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" task
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h0m9s

Stress evidence (decision 0616): the program std/evidence/task/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: task stress: 101709376 operations by 64 tasks in 30 min, 0 mismatches, rss 14 MB at the mark (median of minutes 15 to 17), 14 MB at the end (median of the last 3 minutes; 1.0%), peak 14 MB, pass, 30m0s
- arm64 run 2: task stress: 101769385 operations by 64 tasks in 30 min, 0 mismatches, rss 15 MB at the mark (median of minutes 15 to 17), 15 MB at the end (median of the last 3 minutes; 2.0%), peak 15 MB, pass, 30m0s
- llvm run 1: task stress: 101712660 operations by 64 tasks in 30 min, 0 mismatches, rss 12 MB at the mark (median of minutes 15 to 17), 13 MB at the end (median of the last 3 minutes; 0.0%), peak 13 MB, pass, 30m1s
- llvm run 2: task stress: 101692763 operations by 64 tasks in 30 min, 0 mismatches, rss 13 MB at the mark (median of minutes 15 to 17), 14 MB at the end (median of the last 3 minutes; 6.0%), peak 14 MB, pass, 30m0s
- arm64 --check=all: task stress: 16922068 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 75555366 allocations, 5m0s
