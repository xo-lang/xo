kind: stress
module: backoff
result: pass
commit: 0f6c58d77
source: ed36ec73bdcd2752842d9887f3d675a6bdaf916ac7e4d52ab04f8b6a00b4b691
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" backoff
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m29s

Stress evidence (decision 0616): the program std/evidence/backoff/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: backoff stress: 207460741830 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 7 MB at the end (19.0%), peak 7 MB, pass, 30m0s
- llvm: backoff stress: 182093465940 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (6.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: backoff stress: 17860285695 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 35720571806 allocations, 5m0s
