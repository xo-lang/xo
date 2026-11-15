kind: stress
module: base32
result: pass
commit: 41068182d
source: bfc2c5affdd311f6576402e0b3cf49e8ac20037dce3796aff1a97f321c9b65a6
date: 2026-11-15
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" base32
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m7s

Stress evidence (decision 0616): the program std/evidence/base32/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: base32 stress: 131090299392 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (4.0%), peak 4 MB, pass, 30m0s
- llvm: base32 stress: 75861825216 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (1.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: base32 stress: 9796340736 operations by 64 tasks in 5 min, 0 mismatches, rss 244 MB at the mark, 244 MB at the end (0.0%), peak 244 MB, pass; xo leakcheck: 0 live of 30001294028 allocations, 5m0s
