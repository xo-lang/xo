kind: stress
module: x509
result: pass
commit: (private commit)
source: 7672450cc75806c04667e393ecd117bf9cf492d7aa5c375cf33f8ef7ea07286a
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" x509
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h2m23s

Stress evidence (decision 0616): the program std/evidence/x509/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: x509 stress: 107545969 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark (median of minutes 15 to 17), 6 MB at the end (median of the last 3 minutes; 0.0%), peak 6 MB, pass, 30m0s
- arm64 run 2: x509 stress: 132618792 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark (median of minutes 15 to 17), 7 MB at the end (median of the last 3 minutes; 0.0%), peak 7 MB, pass, 30m0s
- llvm run 1: x509 stress: 112721264 operations by 64 tasks in 30 min, 0 mismatches, rss 9 MB at the mark (median of minutes 15 to 17), 9 MB at the end (median of the last 3 minutes; 1.0%), peak 9 MB, pass, 30m0s
- llvm run 2: x509 stress: 146063751 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark (median of minutes 15 to 17), 7 MB at the end (median of the last 3 minutes; 0.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: x509 stress: 9823507 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 861570359 allocations, 5m1s
