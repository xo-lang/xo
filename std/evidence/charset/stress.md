kind: stress
module: charset
result: pass
commit: 0f6c58d77
source: 0406c05bb9ad615c3799e31356da05ce77f23d292f7a1e605836330c588ed86c
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" charset
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m28s

Stress evidence (decision 0616): the program std/evidence/charset/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: charset stress: 46689052928 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 5 MB at the end (30.0%), peak 5 MB, pass, 30m0s
- llvm: charset stress: 44267338240 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (1.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: charset stress: 3510793728 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 14372312114 allocations, 5m1s
