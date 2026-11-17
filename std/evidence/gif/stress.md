kind: stress
module: gif
result: pass
commit: 139831fb9
source: 215a7b62f899bead7b3204eeae406fdcb8eeb60adbb8736bce24382341e010c4
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" gif
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m41s

Stress evidence (decision 0616): the program std/evidence/gif/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: gif stress: 131906816 operations by 64 tasks in 30 min, 0 mismatches, rss 35 MB at the mark, 27 MB at the end (-22.0%), peak 42 MB, pass, 30m1s
- llvm: gif stress: 177328384 operations by 64 tasks in 30 min, 0 mismatches, rss 31 MB at the mark, 30 MB at the end (-4.0%), peak 31 MB, pass, 30m0s
- arm64 --check=all: gif stress: 3375104 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 504323410 allocations, 5m2s
