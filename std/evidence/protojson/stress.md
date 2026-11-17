kind: stress
module: protojson
result: pass
commit: a9fd5864b
source: f6b7c2b16bab5db6c7b459f4fd22011a507d18c5cf72de6498af6997a3bc4eb8
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" protojson
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m42s

Stress evidence (decision 0616): the program std/evidence/protojson/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: protojson stress: 1061862 operations by 64 tasks in 30 min, 0 mismatches, rss 13 MB at the mark, 12 MB at the end (-5.0%), peak 13 MB, pass, 30m1s
- llvm: protojson stress: 1167084 operations by 64 tasks in 30 min, 0 mismatches, rss 12 MB at the mark, 11 MB at the end (-8.0%), peak 13 MB, pass, 30m0s
- arm64 --check=all: protojson stress: 58608 operations by 64 tasks in 5 min, 0 mismatches, rss 209 MB at the mark, 209 MB at the end (0.0%), peak 209 MB, pass; xo leakcheck: 0 live of 4766749931 allocations, 5m3s
