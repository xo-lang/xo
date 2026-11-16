kind: stress
module: deque
result: pass
commit: 52efc3d15
source: f17015181d59f9bb234d9c0e9dbd788aaa75883cc549df60d555ea829cb6c49a
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" deque
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m9s

Stress evidence (decision 0616): the program std/evidence/deque/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: deque stress: 452490791808 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 9 MB at the end (18.0%), peak 9 MB, pass, 30m0s
- llvm: deque stress: 414991235560 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 7 MB at the end (2.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: deque stress: 21126734222 operations by 64 tasks in 5 min, 0 mismatches, rss 262 MB at the mark, 262 MB at the end (0.0%), peak 262 MB, pass; xo leakcheck: 0 live of 21401741572 allocations, 5m0s
