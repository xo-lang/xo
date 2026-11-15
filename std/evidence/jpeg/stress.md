kind: stress
module: jpeg
result: pass
commit: d6d6b5c8f
source: 5f0d82e205789ddcdafa775bcd5797960a6fca94b5a193128bcba9c40c8e83e2
date: 2026-11-15
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" jpeg
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m20s

Stress evidence (decision 0616): the program std/evidence/jpeg/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: jpeg stress: 142505856 operations by 64 tasks in 30 min, 0 mismatches, rss 57 MB at the mark, 58 MB at the end (1.0%), peak 58 MB, pass, 30m0s
- llvm: jpeg stress: 185220864 operations by 64 tasks in 30 min, 0 mismatches, rss 53 MB at the mark, 45 MB at the end (-14.0%), peak 53 MB, pass, 30m0s
- arm64 --check=all: jpeg stress: 18050688 operations by 64 tasks in 5 min, 0 mismatches, rss 1293 MB at the mark, 1306 MB at the end (1.0%), peak 1306 MB, pass; xo leakcheck: 0 live of 499840810 allocations, 5m1s
