kind: stress
module: tabwriter
result: pass
commit: 0f6c58d77
source: 3c8c7d20fed3259169c9750ba55aabe00a7dc7c4e8dd820355f9d4221827d6a0
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" tabwriter
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m5s

Stress evidence (decision 0616): the program std/evidence/tabwriter/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: tabwriter stress: 7994133120 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: tabwriter stress: 4600717824 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (3.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: tabwriter stress: 402616384 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 8988750780 allocations, 5m1s
