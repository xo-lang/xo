kind: stress
module: timego
result: pass
commit: 224654253
source: 78fa66e93943b04c6493f4560aec422d9f5584624f8984e9e6780ea9a3fcd2c2
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" timego
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m15s

Stress evidence (decision 0616): the program std/evidence/timego/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: timego stress: 14656751104 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark, 3 MB at the end (0.0%), peak 3 MB, pass, 30m0s
- llvm: timego stress: 13585743872 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (1.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: timego stress: 802227712 operations by 64 tasks in 5 min, 0 mismatches, rss 52 MB at the mark, 55 MB at the end (4.0%), peak 55 MB, pass; xo leakcheck: 0 live of 7533420122 allocations, 5m1s
