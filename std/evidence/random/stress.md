kind: stress
module: random
result: pass
commit: 0f2f1d2b1
source: 472f5da38e7e959093060f9f3163e86a91c3fc3d44b8a80aef62c6d59810407b
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" random
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m57s

Stress evidence (decision 0616): the program std/evidence/random/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: random stress: 12092204068 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (2.0%), peak 4 MB, pass, 30m2s
- llvm: random stress: 10139009524 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (2.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: random stress: 1304125704 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 4156794218 allocations, 5m10s
