kind: stress
module: heap
result: pass
commit: 52efc3d15
source: e168e8eed85b199d9a02ee6d0c6f1982fb69cc99d3569c5740a630a5882a0cf0
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" heap
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m11s

Stress evidence (decision 0616): the program std/evidence/heap/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: heap stress: 204882564690 operations by 64 tasks in 30 min, 0 mismatches, rss 9 MB at the mark, 9 MB at the end (3.0%), peak 9 MB, pass, 30m0s
- llvm: heap stress: 151893768690 operations by 64 tasks in 30 min, 0 mismatches, rss 8 MB at the mark, 9 MB at the end (3.0%), peak 9 MB, pass, 30m0s
- arm64 --check=all: heap stress: 24589545960 operations by 64 tasks in 5 min, 0 mismatches, rss 910 MB at the mark, 923 MB at the end (1.0%), peak 923 MB, pass; xo leakcheck: 0 live of 6689064360 allocations, 5m0s
