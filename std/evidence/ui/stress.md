kind: stress
module: ui
result: pass
commit: 8f94a12ad
source: 1286de45e055e5c3f813c331ad007ed709d061d5425c552eaaa71cc19fba2894
date: 2026-11-20
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" ui
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h2m36s

Stress evidence (decision 0616): the program std/evidence/ui/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: ui stress: 7290368 syncs of 10935910 rounds by 64 clients in 30 min (3381 reloads), every Msg applied once, every replica converged, rss 87 MB at the mark (median of minutes 15 to 17), 81 MB at the end (median of the last 3 minutes; -7.0%), peak 103 MB, pass, 30m0s
- arm64 run 2: ui stress: 9972379 syncs of 14961898 rounds by 64 clients in 30 min (1920 reloads), every Msg applied once, every replica converged, rss 75 MB at the mark (median of minutes 15 to 17), 82 MB at the end (median of the last 3 minutes; 9.0%), peak 101 MB, pass, 30m2s
- llvm run 1: ui stress: 7334310 syncs of 11002076 rounds by 64 clients in 30 min (3587 reloads), every Msg applied once, every replica converged, rss 87 MB at the mark (median of minutes 15 to 17), 77 MB at the end (median of the last 3 minutes; -12.0%), peak 102 MB, pass, 30m30s
- llvm run 2: ui stress: 10103372 syncs of 15158183 rounds by 64 clients in 30 min (2012 reloads), every Msg applied once, every replica converged, rss 75 MB at the mark (median of minutes 15 to 17), 81 MB at the end (median of the last 3 minutes; 7.0%), peak 108 MB, pass, 30m1s
- arm64 --check=all: ui stress: 1113394 syncs of 1670730 rounds by 64 clients in 5 min (597 reloads), every Msg applied once, every replica converged, rss not sampled, pass; xo leakcheck: 0 live of 2144131469 allocations, 5m0s
