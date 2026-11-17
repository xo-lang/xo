kind: stress
module: protowkt
result: pass
commit: 224654253
source: 6f74b15e5d07df13ee5dab859c1fa47e3b0578e96d325f95661bc015ed5a36db
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" protowkt
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 32m19s

Stress evidence (decision 0616): the program std/evidence/protowkt/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: protowkt stress: 26907114 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (1.0%), peak 4 MB, pass, 30m1s
- llvm: protowkt stress: 26329992 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (1.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: protowkt stress: 532714 operations by 64 tasks in 5 min, 0 mismatches, rss 153 MB at the mark, 153 MB at the end (0.0%), peak 153 MB, pass; xo leakcheck: 0 live of 2249742308 allocations, 5m2s
