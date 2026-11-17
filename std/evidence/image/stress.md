kind: stress
module: image
result: pass
commit: 0f6c58d77
source: 7c927933e66698de2e6b0ba3efbbb523cef4a1b0fb11209429cc1b51bd881c0c
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" image
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m27s

Stress evidence (decision 0616): the program std/evidence/image/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: image stress: 44171966304 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: image stress: 70547463936 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (1.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: image stress: 5859604512 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 2441502182 allocations, 5m0s
