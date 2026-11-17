kind: stress
module: protowire
result: pass
commit: 0f6c58d77
source: c08365bfae406e57ad1c63ca85df9c233eeed723b061e62a966378a289f83935
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" protowire
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m26s

Stress evidence (decision 0616): the program std/evidence/protowire/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: protowire stress: 2155610368 operations by 64 tasks in 30 min, 0 mismatches, rss 55 MB at the mark, 55 MB at the end (0.0%), peak 55 MB, pass, 30m0s
- llvm: protowire stress: 1850808448 operations by 64 tasks in 30 min, 0 mismatches, rss 65 MB at the mark, 65 MB at the end (0.0%), peak 65 MB, pass, 30m0s
- arm64 --check=all: protowire stress: 232319104 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 1285015334 allocations, 5m1s
