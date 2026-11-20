kind: stress
module: ui
result: pass
commit: fe31f0d49
source: 73b026ac0768899a893448cb3ec4cd02c1fb824214fc05b24a1dad5475409df8
date: 2026-11-20
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" ui
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h38m52s

Stress evidence (decision 0616): the program std/evidence/ui/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: ui stress: 5168727 syncs of 7753680 rounds by 64 clients in 30 min (690 reloads), every Msg applied once, every replica converged, rss 68 MB at the mark (median of minutes 15 to 17), 68 MB at the end (median of the last 3 minutes; 0.0%), peak 70 MB, pass, 30m0s
- arm64 run 2: ui stress: 5866500 syncs of 8800973 rounds by 64 clients in 30 min (790 reloads), every Msg applied once, every replica converged, rss 66 MB at the mark (median of minutes 15 to 17), 67 MB at the end (median of the last 3 minutes; 2.0%), peak 73 MB, pass, 30m30s
- llvm run 1: ui stress: 5563418 syncs of 8345718 rounds by 64 clients in 30 min (755 reloads), every Msg applied once, every replica converged, rss 67 MB at the mark (median of minutes 15 to 17), 67 MB at the end (median of the last 3 minutes; 1.0%), peak 75 MB, pass, 30m0s
- llvm run 2: ui stress: 7778553 syncs of 11669266 rounds by 64 clients in 30 min (671 reloads), every Msg applied once, every replica converged, rss 67 MB at the mark (median of minutes 15 to 17), 74 MB at the end (median of the last 3 minutes; 10.0%), peak 76 MB, FAIL (memory grew), 30m0s
- llvm run 3: ui stress: 12773792 syncs of 19160988 rounds by 64 clients in 30 min (9 reloads), every Msg applied once, every replica converged, rss 73 MB at the mark (median of minutes 15 to 17), 77 MB at the end (median of the last 3 minutes; 7.0%), peak 78 MB, pass, 30m0s
- arm64 --check=all: ui stress: 383897 syncs of 576303 rounds by 64 clients in 5 min (141 reloads), every Msg applied once, every replica converged, rss not sampled, pass; xo leakcheck: 0 live of 745131448 allocations, 5m32s
