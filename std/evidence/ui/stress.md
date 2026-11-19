kind: stress
module: ui
result: pass
commit: 2eca139d6
source: ac289646d9612f4e7608e8ce03e2218bff5c1eeac568a03fea1644d8c02812ab
date: 2026-11-19
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64 -check-minutes 0 -host "linux/arm64" ui
measure: 64 tasks for 30 min on each of arm64, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 1h0m4s

Stress evidence (decision 0616): the program std/evidence/ui/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: ui stress: 23374010 syncs of 35058176 rounds by 64 clients in 30 min (1619 reloads), every Msg applied once, every replica converged, rss 94 MB at the mark (median of minutes 15 to 17), 96 MB at the end (median of the last 3 minutes; 2.0%), peak 97 MB, pass, 30m0s
- arm64 run 2: ui stress: 26705248 syncs of 40055087 rounds by 64 clients in 30 min (1103 reloads), every Msg applied once, every replica converged, rss 104 MB at the mark (median of minutes 15 to 17), 107 MB at the end (median of the last 3 minutes; 2.0%), peak 110 MB, pass, 30m0s
kind: stress
module: ui
result: pass
commit: 2eca139d6
source: ac289646d9612f4e7608e8ce03e2218bff5c1eeac568a03fea1644d8c02812ab
date: 2026-11-19
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends llvm -check-minutes 0 -host "linux/arm64" ui
measure: 64 tasks for 30 min on each of llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 1h1m25s

Stress evidence (decision 0616): the program std/evidence/ui/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- llvm run 1: ui stress: 28674215 syncs of 43007878 rounds by 64 clients in 30 min (820 reloads), every Msg applied once, every replica converged, rss 139 MB at the mark (median of minutes 15 to 17), 141 MB at the end (median of the last 3 minutes; 2.0%), peak 151 MB, pass, 30m0s
- llvm run 2: ui stress: 25060914 syncs of 37588862 rounds by 64 clients in 30 min (1044 reloads), every Msg applied once, every replica converged, rss 118 MB at the mark (median of minutes 15 to 17), 120 MB at the end (median of the last 3 minutes; 1.0%), peak 124 MB, pass, 30m0s
