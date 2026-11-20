kind: stress
module: ui
result: pass
commit: f3276a733
source: 343da67d2b9099bf40f28c7fc6c9748419238f2fb53cf360ffdbef282631ee4d
date: 2026-11-20
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" ui
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h2m49s

Stress evidence (decision 0616): the program std/evidence/ui/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: ui stress: 10914114 syncs of 16373497 rounds by 64 clients in 30 min (3345 reloads), every Msg applied once, every replica converged, rss 110 MB at the mark (median of minutes 15 to 17), 96 MB at the end (median of the last 3 minutes; -12.0%), peak 112 MB, pass, 30m0s
- arm64 run 2: ui stress: 9558167 syncs of 14340201 rounds by 64 clients in 30 min (2821 reloads), every Msg applied once, every replica converged, rss 89 MB at the mark (median of minutes 15 to 17), 89 MB at the end (median of the last 3 minutes; 0.0%), peak 108 MB, pass, 30m0s
- llvm run 1: ui stress: 11122150 syncs of 16685732 rounds by 64 clients in 30 min (3574 reloads), every Msg applied once, every replica converged, rss 100 MB at the mark (median of minutes 15 to 17), 96 MB at the end (median of the last 3 minutes; -4.0%), peak 112 MB, pass, 30m0s
- llvm run 2: ui stress: 9794445 syncs of 14694475 rounds by 64 clients in 30 min (2990 reloads), every Msg applied once, every replica converged, rss 88 MB at the mark (median of minutes 15 to 17), 88 MB at the end (median of the last 3 minutes; -0.0%), peak 108 MB, pass, 30m30s
- arm64 --check=all: ui stress: 1297340 syncs of 1946862 rounds by 64 clients in 5 min (519 reloads), every Msg applied once, every replica converged, rss not sampled, pass; xo leakcheck: 0 live of 2508611801 allocations, 5m30s
