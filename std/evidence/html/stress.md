kind: stress
module: html
result: pass
commit: eb7b7300e
source: 5747b639da584a7ec48020992f3b0645191623cb176c4d06ef908f4d699be444
date: 2026-11-14
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" html
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m15s

Stress evidence (decision 0616): the program std/evidence/html/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: html stress: 152585906688 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (1.0%), peak 6 MB, pass, 30m0s
- llvm: html stress: 96149453376 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: html stress: 15747303744 operations by 64 tasks in 5 min, 0 mismatches, rss 132 MB at the mark, 132 MB at the end (0.0%), peak 132 MB, pass; xo leakcheck: 0 live of 15255201022 allocations, 5m0s
