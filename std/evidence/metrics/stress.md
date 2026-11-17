kind: stress
module: metrics
result: pass
commit: a9fd5864b
source: 9bce8a937763e6187d2abf6837fa669ca266859061fcf9fbcf48e0c845e14065
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" metrics
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m8s

Stress evidence (decision 0616): the program std/evidence/metrics/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: metrics stress: 278787236 operations by 64 tasks in 30 min, 0 mismatches, rss 16 MB at the mark, 16 MB at the end (0.0%), peak 17 MB, pass, 30m0s
- llvm: metrics stress: 253117452 operations by 64 tasks in 30 min, 0 mismatches, rss 16 MB at the mark, 17 MB at the end (7.0%), peak 17 MB, pass, 30m0s
- arm64 --check=all: metrics stress: 3820278 operations by 64 tasks in 5 min, 0 mismatches, rss 133 MB at the mark, 133 MB at the end (0.0%), peak 133 MB, pass; xo leakcheck: 0 live of 1156181798 allocations, 5m0s
