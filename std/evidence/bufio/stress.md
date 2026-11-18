kind: stress
module: bufio
result: pass
commit: 9f490a22d
source: 13cbd58b5b5557a2df32a7e72254609d0933160feed319f48e32f60d6c9bf9a4
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" bufio
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m32s

Stress evidence (decision 0616): the program std/evidence/bufio/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: bufio stress: 11077604788 operations by 64 tasks in 30 min, 0 mismatches, rss 9 MB at the mark (median of minutes 5 to 7), 9 MB at the end (median of the last 3 minutes; 4.0%), peak 9 MB, pass, 30m1s
- llvm: bufio stress: 9314869228 operations by 64 tasks in 30 min, 0 mismatches, rss not sampled, pass, 30m0s
- arm64 --check=all: bufio stress: 1639217556 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 3891165435 allocations, 5m0s
