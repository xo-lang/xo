kind: stress
module: migrate
result: pass
commit: ef0cb20d5
source: 5c259261d62f2f1bdc3a039722441a1c78597c8c34aa84967301033aa79255d6
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" migrate
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h0m57s

Stress evidence (decision 0616): the program std/evidence/migrate/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: migrate stress: 824761644 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 1.0%), peak 5 MB, pass, 30m0s
- arm64 run 2: migrate stress: 895740804 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 1.0%), peak 5 MB, pass, 30m0s
- llvm run 1: migrate stress: 837362554 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark (median of minutes 15 to 17), 7 MB at the end (median of the last 3 minutes; 0.0%), peak 7 MB, pass, 30m0s
- llvm run 2: migrate stress: 954981916 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark (median of minutes 15 to 17), 6 MB at the end (median of the last 3 minutes; 1.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: migrate stress: 36498140 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 4825491571 allocations, 5m0s
