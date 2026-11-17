kind: stress
module: xmlcodec
result: pass
commit: 224654253
source: b01096b6b9bb64d1454849936dd115c187d34647ac34d698ec8d6d6f91801f2b
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" xmlcodec
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m26s

Stress evidence (decision 0616): the program std/evidence/xmlcodec/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: xmlcodec stress: 1161211200 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- llvm: xmlcodec stress: 656891400 operations by 64 tasks in 30 min, 0 mismatches, rss 8 MB at the mark, 8 MB at the end (3.0%), peak 8 MB, pass, 30m0s
- arm64 --check=all: xmlcodec stress: 54081000 operations by 64 tasks in 5 min, 0 mismatches, rss 137 MB at the mark, 137 MB at the end (0.0%), peak 137 MB, pass; xo leakcheck: 0 live of 3449198056 allocations, 5m1s
