kind: stress
module: grpchealth
result: pass
commit: 3979131cf
source: b10c860525d4bd45eb55ddb0530f1975430f899839f60f0ba72a7b1787963625
date: 2026-11-19
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" grpchealth
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h0m49s

Stress evidence (decision 0616): the program std/evidence/grpchealth/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: grpchealth stress: 41722590 rounds by 64 tasks in 30 min, every answer as expected, rss 64 MB at the mark (median of minutes 15 to 17), 67 MB at the end (median of the last 3 minutes; 4.0%), peak 67 MB, pass, 30m0s
- arm64 run 2: grpchealth stress: 36061740 rounds by 64 tasks in 30 min, every answer as expected, rss 59 MB at the mark (median of minutes 15 to 17), 62 MB at the end (median of the last 3 minutes; 6.0%), peak 63 MB, pass, 30m30s
- llvm run 1: grpchealth stress: 38607745 rounds by 64 tasks in 30 min, every answer as expected, rss 64 MB at the mark (median of minutes 15 to 17), 66 MB at the end (median of the last 3 minutes; 3.0%), peak 68 MB, pass, 30m0s
- llvm run 2: grpchealth stress: 34093996 rounds by 64 tasks in 30 min, every answer as expected, rss 58 MB at the mark (median of minutes 15 to 17), 60 MB at the end (median of the last 3 minutes; 4.0%), peak 62 MB, pass, 30m0s
- arm64 --check=all: grpchealth stress: 1278620 rounds by 64 tasks in 5 min, every answer as expected, rss not sampled, pass; xo leakcheck: 0 live of 1176660677 allocations, 5m0s
