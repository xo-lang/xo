kind: stress
module: compress
result: pass
commit: (private commit)
source: f51ab22d3d6118fe4cecf839d9a848b5336c93e87c5a754aa40f25b5d9792ec5
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" compress
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h1m25s

Stress evidence (decision 0616): the program std/evidence/compress/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: compress stress: 10989000 operations by 64 tasks in 30 min, 0 mismatches, rss 84 MB at the mark (median of minutes 15 to 17), 84 MB at the end (median of the last 3 minutes; 0.0%), peak 88 MB, pass, 30m20s
- arm64 run 2: compress stress: 10911600 operations by 64 tasks in 30 min, 0 mismatches, rss 84 MB at the mark (median of minutes 15 to 17), 83 MB at the end (median of the last 3 minutes; -1.0%), peak 86 MB, pass, 30m16s
- llvm run 1: compress stress: 11671200 operations by 64 tasks in 30 min, 0 mismatches, rss 81 MB at the mark (median of minutes 15 to 17), 82 MB at the end (median of the last 3 minutes; 1.0%), peak 86 MB, pass, 30m20s
- llvm run 2: compress stress: 11822400 operations by 64 tasks in 30 min, 0 mismatches, rss 85 MB at the mark (median of minutes 15 to 17), 84 MB at the end (median of the last 3 minutes; -1.0%), peak 87 MB, pass, 30m12s
- arm64 --check=all: compress stress: 1288800 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 136673225 allocations, 5m34s
