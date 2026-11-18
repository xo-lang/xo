kind: stress
module: json
result: pass
commit: (private commit)
source: d0952fd5fd6bc8dcf9f970f6cbd9ebd55a1e82f3f1dab509dce66e772351cf91
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" json
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h1m2s

Stress evidence (decision 0616): the program std/evidence/json/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: json stress: 2855036480 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 1.0%), peak 5 MB, pass, 30m0s
- arm64 run 2: json stress: 2860709120 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark (median of minutes 15 to 17), 4 MB at the end (median of the last 3 minutes; 1.0%), peak 4 MB, pass, 30m0s
- llvm run 1: json stress: 3032848320 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark (median of minutes 15 to 17), 4 MB at the end (median of the last 3 minutes; 1.0%), peak 4 MB, pass, 30m0s
- llvm run 2: json stress: 2794612480 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark (median of minutes 15 to 17), 5 MB at the end (median of the last 3 minutes; 1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: json stress: 137467840 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 2148364875 allocations, 5m0s
