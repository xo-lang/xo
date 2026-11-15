kind: stress
module: fnv
result: pass
commit: 783663cf2
source: 5f34cd0ac2005b37cf77ebf324b2f8d539b5827efe52c87aebbb39fc17218cba
date: 2026-11-14
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64 host 2" fnv
check: go run ./scripts/stdproven stress -check-only -check-minutes 5 -tasks 64 -host "linux/arm64 host 2" fnv (commit 4b86ae828, 2026-11-15)
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h0m11s

Stress evidence (decision 0616): the program std/evidence/fnv/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: fnv stress: 14734244736 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- llvm: fnv stress: 19889543232 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: fnv stress: 4493429568 operations by 64 tasks in 5 min, 0 mismatches, rss 1025 MB at the mark, 1030 MB at the end (1.0%), peak 1030 MB, pass; xo leakcheck: 0 live of 5991239946 allocations, 5m0s
