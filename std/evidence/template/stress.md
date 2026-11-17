kind: stress
module: template
result: pass
commit: 224654253
source: ac4c3d141156db3ab1bca9881fe29f81ba5d025788bf9578b494ad58b4b16adf
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" template
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 32m34s

Stress evidence (decision 0616): the program std/evidence/template/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: template stress: 655476000 operations by 64 tasks in 30 min, 0 mismatches, rss 8 MB at the mark, 8 MB at the end (2.0%), peak 8 MB, pass, 30m0s
- llvm: template stress: 623740400 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 8 MB at the end (3.0%), peak 8 MB, pass, 30m0s
- arm64 --check=all: template stress: 32380000 operations by 64 tasks in 5 min, 0 mismatches, rss 192 MB at the mark, 191 MB at the end (-0.0%), peak 192 MB, pass; xo leakcheck: 0 live of 1790452623 allocations, 5m1s
