kind: stress
module: publicsuffix
result: pass
commit: 0f2f1d2b1
source: 04450658eed33d05e18b444831a7f52fd0fe19129103f2e0a0f1278a8b81b873
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" publicsuffix
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m22s

Stress evidence (decision 0616): the program std/evidence/publicsuffix/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: publicsuffix stress: 27740779392 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (0.0%), peak 4 MB, pass, 30m1s
- llvm: publicsuffix stress: 34751308800 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark, 3 MB at the end (0.0%), peak 3 MB, pass, 30m0s
- arm64 --check=all: publicsuffix stress: 3047176064 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 2094933832 allocations, 5m3s
