kind: stress
module: debug
result: pass
commit: db1e50d9c
source: 708b98a31dc79561f2b28bbed5c49d61bdb38d01c25a1981785566a44d0e2a8c
date: 2026-11-18
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64" debug
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 2h5m20s

Stress evidence (decision 0616): the program std/evidence/debug/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: debug stress: 1488775047 operations by 64 tasks in 30 min, 0 mismatches, rss 54 MB at the mark (median of minutes 15 to 17), 54 MB at the end (median of the last 3 minutes; -0.0%), peak 64 MB, pass, 30m0s
- arm64 run 2: debug stress: 1445826092 operations by 64 tasks in 30 min, 0 mismatches, rss 59 MB at the mark (median of minutes 15 to 17), 55 MB at the end (median of the last 3 minutes; -7.0%), peak 66 MB, pass, 30m1s
- llvm run 1: debug stress: 367483137 operations by 64 tasks in 30 min, 0 mismatches, rss 49 MB at the mark (median of minutes 15 to 17), 52 MB at the end (median of the last 3 minutes; 6.0%), peak 57 MB, pass, 30m0s
- llvm run 2: debug stress: 384638588 operations by 64 tasks in 30 min, 0 mismatches, rss 44 MB at the mark (median of minutes 15 to 17), 45 MB at the end (median of the last 3 minutes; 3.0%), peak 52 MB, pass, 30m0s
- arm64 --check=all: debug stress: 207788947 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 4548687452 allocations, 5m0s
