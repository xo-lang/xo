kind: stress
module: time
result: pass
commit: (private commit)
source: 5b9b59bec1ee475b4128995f8fb0dfcb32deb35b0f1da6adbc3857f61754e0b1
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" time
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h0m31s

Stress evidence (decision 0616): the program std/evidence/time/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: time stress: 21094119936 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark (median of minutes 15 to 17), 4 MB at the end (median of the last 3 minutes; 0.0%), peak 4 MB, pass, 30m0s
- arm64 run 2: time stress: 25368193024 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 0.0%), peak 5 MB, pass, 30m0s
- llvm run 1: time stress: 21053908992 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark (median of minutes 15 to 17), 3 MB at the end (median of the last 3 minutes; 0.0%), peak 3 MB, pass, 30m1s
- llvm run 2: time stress: 25966958080 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark (median of minutes 15 to 17), 3 MB at the end (median of the last 3 minutes; 0.0%), peak 3 MB, pass, 30m0s
- arm64 --check=all: time stress: 1935733760 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 1841215799 allocations, 5m2s
