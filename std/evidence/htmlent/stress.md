kind: stress
module: htmlent
result: pass
commit: 52efc3d15
source: 4f9a032d3b0a0ee5bf26a6615f0fe4876054f639cda3edb92e682393cb61a702
date: 2026-11-16
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" htmlent
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m26s

Stress evidence (decision 0616): the program std/evidence/htmlent/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: htmlent stress: 90266092608 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: htmlent stress: 73281522240 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: htmlent stress: 11805733824 operations by 64 tasks in 5 min, 0 mismatches, rss 62 MB at the mark, 64 MB at the end (3.0%), peak 64 MB, pass; xo leakcheck: 0 live of 7809001553 allocations, 5m0s
