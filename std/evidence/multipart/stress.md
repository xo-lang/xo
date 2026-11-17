kind: stress
module: multipart
result: pass
commit: 224654253
source: 907bb4b11915093de24d728706fe3cfccee080f056471a2c6160ebee3d8ef2b1
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" multipart
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m52s

Stress evidence (decision 0616): the program std/evidence/multipart/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: multipart stress: 6774706 forms by 64 tasks in 30 min (515603 MB written and read back 3 ways), 0 differences, rss 84 MB at the mark, 89 MB at the end (6.0%), peak 92 MB, pass, 30m1s
- llvm: multipart stress: 4741785 forms by 64 tasks in 30 min (360878 MB written and read back 3 ways), 0 differences, rss 89 MB at the mark, 96 MB at the end (7.0%), peak 99 MB, pass, 30m0s
- arm64 --check=all: multipart stress: 336550 forms by 64 tasks in 5 min (25615 MB written and read back 3 ways), 0 differences, rss 444 MB at the mark, 453 MB at the end (2.0%), peak 453 MB, pass; xo leakcheck: 0 live of 352670668 allocations, 5m1s
