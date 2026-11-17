kind: stress
module: log
result: pass
commit: 0f6c58d77
source: 9980e5649ada9d0f34ff184ecc55d0b585a40fcb83e64b1e46c07b95fe4c6c04
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" log
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m23s

Stress evidence (decision 0616): the program std/evidence/log/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: log stress: 1064460300 operations by 64 tasks in 30 min, 0 mismatches, rss 12 MB at the mark, 14 MB at the end (14.0%), peak 14 MB, pass, 30m0s
- llvm: log stress: 1124669700 operations by 64 tasks in 30 min, 0 mismatches, rss 11 MB at the mark, 15 MB at the end (28.0%), peak 15 MB, pass, 30m0s
- arm64 --check=all: log stress: 75961200 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 2801594401 allocations, 5m0s
