kind: stress
module: rate
result: pass
commit: 224654253
source: 040d9b549cc5779ce322eb5eb777949c947f038704d1cd7783e2a5b6ce6ce104
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" rate
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m26s

Stress evidence (decision 0616): the program std/evidence/rate/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: rate stress: 72922515845 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (1.0%), peak 6 MB, pass, 30m0s
- llvm: rate stress: 57199759923 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 7 MB at the end (2.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: rate stress: 5262082434 operations by 64 tasks in 5 min, 0 mismatches, rss 310 MB at the mark, 319 MB at the end (3.0%), peak 319 MB, pass; xo leakcheck: 0 live of 5673756329 allocations, 5m1s
