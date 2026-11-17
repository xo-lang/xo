kind: stress
module: trace
result: pass
commit: 224654253
source: d381fe4fec04bd8b168174a4ef63a7e03a25da81333729b3b1797bfcaf6e1895
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" trace
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m33s

Stress evidence (decision 0616): the program std/evidence/trace/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: trace stress: 62817683 spans recorded and 1834087887 read back by 64 tasks in 30 min, 0 promises broken, rss 18 MB at the mark, 20 MB at the end (10.0%), peak 21 MB, pass, 30m0s
- llvm: trace stress: 61821753 spans recorded and 1805218235 read back by 64 tasks in 30 min, 0 promises broken, rss 22 MB at the mark, 21 MB at the end (-4.0%), peak 22 MB, pass, 30m0s
- arm64 --check=all: trace stress: 3169584 spans recorded and 92543340 read back by 64 tasks in 5 min, 0 promises broken, rss 144 MB at the mark, 152 MB at the end (5.0%), peak 154 MB, pass; xo leakcheck: 0 live of 1173340144 allocations, 5m0s
