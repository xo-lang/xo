kind: stress
module: big
result: pass
commit: 0f6c58d77
source: 1a87f6298484b5bf1b8e61bb2a882f2c4391851af37195fcca4061668f0853b2
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" big
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m17s

Stress evidence (decision 0616): the program std/evidence/big/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: big stress: 14113344768 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: big stress: 13751953344 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (2.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: big stress: 516341952 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 18144579199 allocations, 5m0s
