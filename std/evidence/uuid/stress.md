kind: stress
module: uuid
result: pass
commit: f9aacc150
source: c3cacf57c4b2235e9fff8cf0544f04a5c6fba12342c82c322019488bc35be316
date: 2026-11-15
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" uuid
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m14s

Stress evidence (decision 0616): the program std/evidence/uuid/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: uuid stress: 262438468380 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 4 MB at the end (-4.0%), peak 5 MB, pass, 30m0s
- llvm: uuid stress: 327992028012 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: uuid stress: 54301437432 operations by 64 tasks in 5 min, 0 mismatches, rss 44 MB at the mark, 44 MB at the end (0.0%), peak 44 MB, pass; xo leakcheck: 0 live of 95987389912 allocations, 5m0s
