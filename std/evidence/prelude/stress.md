kind: stress
module: prelude
result: pass
commit: 8ea9d8269
source: ed3dae2628567178f45357a47753a565525c459097934ce44ec35fcc2a0621d7
date: 2026-11-19
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64" prelude
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 2h5m8s

Stress evidence (decision 0616): the program std/evidence/prelude/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: prelude stress: 103412763648 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 0.0%), peak 5 MB, pass, 30m0s
- arm64 run 2: prelude stress: 106186880512 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 0.0%), peak 5 MB, pass, 30m0s
- llvm run 1: prelude stress: 105832576512 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 1.0%), peak 5 MB, pass, 30m0s
- llvm run 2: prelude stress: 105909377536 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 2.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: prelude stress: 5895502336 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 39571695356 allocations, 5m0s
