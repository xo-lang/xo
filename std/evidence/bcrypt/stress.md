kind: stress
module: bcrypt
result: pass
commit: 0f6c58d77
source: 65b06f6a1cb7021bcf1a4f0760a23078339b43d06364f11ac6258bca84bf51ee
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" bcrypt
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m16s

Stress evidence (decision 0616): the program std/evidence/bcrypt/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: bcrypt stress: 3336691 passwords by 64 tasks in 30 min, 0 identities broken, rss 6 MB at the mark, 6 MB at the end (1.0%), peak 6 MB, pass, 30m0s
- llvm: bcrypt stress: 2973663 passwords by 64 tasks in 30 min, 0 identities broken, rss 6 MB at the mark, 6 MB at the end (1.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: bcrypt stress: 395100 passwords by 64 tasks in 5 min, 0 identities broken, rss not sampled, pass; xo leakcheck: 0 live of 48384297 allocations, 5m1s
