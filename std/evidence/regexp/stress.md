kind: stress
module: regexp
result: pass
commit: 0f6c58d77
source: 008efc3ad36ad39b63b18d87046acd8dc10667ce6ad26d1716e408e813180590
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" regexp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m3s

Stress evidence (decision 0616): the program std/evidence/regexp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: regexp stress: 453210400 operations by 64 tasks in 30 min, 0 mismatches, rss 19 MB at the mark, 17 MB at the end (-8.0%), peak 19 MB, pass, 30m0s
- llvm: regexp stress: 411370800 operations by 64 tasks in 30 min, 0 mismatches, rss 24 MB at the mark, 25 MB at the end (2.0%), peak 26 MB, pass, 30m0s
- arm64 --check=all: regexp stress: 2675600 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 634840296 allocations, 5m5s
