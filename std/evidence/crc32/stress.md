kind: stress
module: crc32
result: pass
commit: 41068182d
source: 15c9a19a15daad089c8b8979d222710f47020953b8c2ce70497aec1771a3f07b
date: 2026-11-15
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" crc32
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m12s

Stress evidence (decision 0616): the program std/evidence/crc32/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: crc32 stress: 53935373184 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (3.0%), peak 5 MB, pass, 30m0s
- llvm: crc32 stress: 81578338176 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (2.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: crc32 stress: 14081263104 operations by 64 tasks in 5 min, 0 mismatches, rss 695 MB at the mark, 695 MB at the end (0.0%), peak 695 MB, pass; xo leakcheck: 0 live of 2310207758 allocations, 5m0s
