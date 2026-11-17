kind: stress
module: embed
result: pass
commit: a14be2e62
source: 5db1ac841a64a96945999b65a42306ad09ed4ca436cc238b35252db0a2741592
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" embed
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m33s

Stress evidence (decision 0616): the program std/evidence/embed/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: embed stress: 1279645196 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (4.0%), peak 4 MB, pass, 30m0s
- llvm: embed stress: 1344570286 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 5 MB at the end (4.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: embed stress: 53516989 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 695932425 allocations, 5m0s
