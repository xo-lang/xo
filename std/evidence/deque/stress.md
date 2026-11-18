kind: stress
module: deque
result: pass
commit: 65c7c7b53
source: f17015181d59f9bb234d9c0e9dbd788aaa75883cc549df60d555ea829cb6c49a
date: 2026-11-18
host: linux/amd64 host
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends amd64,llvm -check-minutes 5 -host "linux/amd64 host" deque
measure: 64 tasks for 30 min on each of amd64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on amd64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m13s

Stress evidence (decision 0616): the program std/evidence/deque/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- amd64: deque stress: 454610469698 operations by 64 tasks in 30 min, 0 mismatches, rss 9 MB at the mark (median of minutes 5 to 7), 9 MB at the end (median of the last 3 minutes; 1.0%), peak 9 MB, pass, 30m0s
- llvm: deque stress: 479924738260 operations by 64 tasks in 30 min, 0 mismatches, rss not sampled, pass, 30m0s
- amd64 --check=all: deque stress: 23481066070 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 23786719548 allocations, 5m0s
