kind: stress
module: sqlite
result: pass
commit: a52ca820a
source: 971880485e9538d26d60947947deb8b4c294ffc3595bd14b4f2340e5acb5bb8f
date: 2026-11-18
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" sqlite
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m33s

Stress evidence (decision 0616): the program std/evidence/sqlite/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: sqlite stress: 8115129 statements by 64 clients over 8 pools in 30 min, 0 mismatches, rss 13 MB at the mark (median of minutes 5 to 7), 13 MB at the end (median of the last 3 minutes; 1.0%), peak 14 MB, pass, 30m0s
- llvm: sqlite stress: 8192187 statements by 64 clients over 8 pools in 30 min, 0 mismatches, rss 13 MB at the mark (median of minutes 5 to 7), 13 MB at the end (median of the last 3 minutes; 1.0%), peak 14 MB, pass, 30m0s
- arm64 --check=all: sqlite stress: 763866 statements by 64 clients over 8 pools in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 7928162 allocations, 5m1s
