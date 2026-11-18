kind: stress
module: term
result: pass
commit: 653c96030
source: 76e2f95e4d3f53f5c173988d84d60da19236595861d8e514b59e9960271da7c9
date: 2026-11-18
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" term
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m26s

Stress evidence (decision 0616): the program std/evidence/term/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: term stress: 1767541943 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark (median of minutes 5 to 7), 6 MB at the end (median of the last 3 minutes; 1.0%), peak 6 MB, pass, 30m0s
- llvm: term stress: 1602852626 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark (median of minutes 5 to 7), 7 MB at the end (median of the last 3 minutes; 1.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: term stress: 100014448 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 10355493969 allocations, 5m1s
