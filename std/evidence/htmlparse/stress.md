kind: stress
module: htmlparse
result: pass
commit: 224654253
source: caa2ebbbe5295ecae4fc68ae0a89aefcbb5d09d0dadc835b1b8162d295bca099
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" htmlparse
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m21s

Stress evidence (decision 0616): the program std/evidence/htmlparse/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: htmlparse stress: 1682559936 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 6 MB at the end (27.0%), peak 6 MB, pass, 30m0s
- llvm: htmlparse stress: 1808369184 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (2.0%), peak 6 MB, pass, 30m0s
- arm64 --check=all: htmlparse stress: 108883392 operations by 64 tasks in 5 min, 0 mismatches, rss 283 MB at the mark, 283 MB at the end (0.0%), peak 283 MB, pass; xo leakcheck: 0 live of 2365945895 allocations, 5m1s
