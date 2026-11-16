kind: stress
module: crypto
result: pass
commit: 1b73055b4
source: 01b80d8d3f039f174066dcfc227741aa9e3cb4c2e37434b5ed4e914ee4de948f
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" crypto
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m22s

Stress evidence (decision 0616): the program std/evidence/crypto/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: crypto stress: 133045800 operations by 64 tasks in 30 min, 0 mismatches, rss 12 MB at the mark, 13 MB at the end (5.0%), peak 13 MB, pass, 30m1s
- llvm: crypto stress: 150379140 operations by 64 tasks in 30 min, 0 mismatches, rss 12 MB at the mark, 13 MB at the end (5.0%), peak 13 MB, pass, 30m0s
- arm64 --check=all: crypto stress: 21814700 operations by 64 tasks in 5 min, 0 mismatches, rss 797 MB at the mark, 801 MB at the end (1.0%), peak 801 MB, pass; xo leakcheck: 0 live of 553831792 allocations, 5m1s
