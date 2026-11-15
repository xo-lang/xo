kind: stress
module: csv
result: pass
commit: 191613523
source: 728c1ed0ac1a3f1926661234020f83f631735f01f06c88547d140fb6b4eb5027
date: 2026-11-15
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" csv
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m8s

Stress evidence (decision 0616): the program std/evidence/csv/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: csv stress: 57707690840 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 6 MB at the end (24.0%), peak 6 MB, pass, 30m0s
- llvm: csv stress: 93657214368 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: csv stress: 6940951360 operations by 64 tasks in 5 min, 0 mismatches, rss 108 MB at the mark, 112 MB at the end (4.0%), peak 112 MB, pass; xo leakcheck: 0 live of 48868049962 allocations, 5m0s
