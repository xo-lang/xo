kind: stress
module: htmltok
result: pass
commit: 224654253
source: cf5697dc818c7ef93b838ac9cb3ad1fe6ac8b0ab67d555e21b425139a33cc611
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" htmltok
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m14s

Stress evidence (decision 0616): the program std/evidence/htmltok/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: htmltok stress: 10475759616 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark, 4 MB at the end (36.0%), peak 4 MB, pass, 30m1s
- llvm: htmltok stress: 8250830848 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (0.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: htmltok stress: 1026443776 operations by 64 tasks in 5 min, 0 mismatches, rss 48 MB at the mark, 48 MB at the end (0.0%), peak 48 MB, pass; xo leakcheck: 0 live of 7265297865 allocations, 5m0s
