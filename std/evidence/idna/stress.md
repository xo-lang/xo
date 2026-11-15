kind: stress
module: idna
result: pass
commit: b16f09732
source: 43b4baadf7fdc861b507d754c17e8964dbf7cf90ebef483c2d75810202155e30
date: 2026-11-15
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" idna
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m21s

Stress evidence (decision 0616): the program std/evidence/idna/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: idna stress: 10582375680 operations by 64 tasks in 30 min, 0 mismatches, rss 2 MB at the mark, 2 MB at the end (2.0%), peak 5 MB, pass, 30m0s
- llvm: idna stress: 22928924416 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: idna stress: 397611264 operations by 64 tasks in 5 min, 0 mismatches, rss 221 MB at the mark, 224 MB at the end (2.0%), peak 224 MB, pass; xo leakcheck: 0 live of 7672577217 allocations, 5m0s
