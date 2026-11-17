kind: stress
module: prototext
result: pass
commit: 65469d283
source: 692b736bc8b38298f0cbfd40ee08f75bb723289bab2e539e8d8daa2da9883e94
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" prototext
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m35s

Stress evidence (decision 0616): the program std/evidence/prototext/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: prototext stress: 405698700 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (4.0%), peak 6 MB, pass, 30m0s
- llvm: prototext stress: 433714380 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 7 MB at the end (2.0%), peak 7 MB, pass, 30m0s
- arm64 --check=all: prototext stress: 51970380 operations by 64 tasks in 5 min, 0 mismatches, rss 252 MB at the mark, 252 MB at the end (0.0%), peak 252 MB, pass; xo leakcheck: 0 live of 1859732897 allocations, 5m1s
