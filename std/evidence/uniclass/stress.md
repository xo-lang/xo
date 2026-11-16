kind: stress
module: uniclass
result: pass
commit: 52efc3d15
source: 87fb24873656d07b207bb28a302d371e8afbc3bba56ba417169d9968e843c66e
date: 2026-11-16
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" uniclass
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m17s

Stress evidence (decision 0616): the program std/evidence/uniclass/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: uniclass stress: 61355768832 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 7 MB at the end (6.0%), peak 7 MB, pass, 30m0s
- llvm: uniclass stress: 57902734848 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 5 MB at the end (-22.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: uniclass stress: 184829952 operations by 64 tasks in 5 min, 0 mismatches, rss 181 MB at the mark, 181 MB at the end (0.0%), peak 181 MB, pass; xo leakcheck: 0 live of 832117895 allocations, 5m0s
