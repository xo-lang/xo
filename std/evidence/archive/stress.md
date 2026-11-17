kind: stress
module: archive
result: pass
commit: 0f6c58d77
source: 3f24b6af9b4c9eea91413ada780bcfa4a867ba1d1d386735136164f2a8b83a84
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" archive
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m51s

Stress evidence (decision 0616): the program std/evidence/archive/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: archive stress: 2050926445 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 8 MB at the end (21.0%), peak 8 MB, pass, 30m0s
- llvm: archive stress: 1824267356 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 7 MB at the end (9.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: archive stress: 148501731 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 2979935007 allocations, 5m0s
