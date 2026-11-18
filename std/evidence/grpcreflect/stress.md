kind: stress
module: grpcreflect
result: pass
commit: b5aa47f1c
source: 2f322b049564c8c2dd55d30a7678107a0bfcb77a8ada0870a3abf25fd82d81b7
date: 2026-11-18
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64" grpcreflect
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 2h6m21s

Stress evidence (decision 0616): the program std/evidence/grpcreflect/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: grpcreflect stress: 107687492 rounds by 64 tasks in 30 min, every answer as expected, rss 93 MB at the mark (median of minutes 15 to 17), 95 MB at the end (median of the last 3 minutes; 2.0%), peak 96 MB, pass, 30m0s
- arm64 run 2: grpcreflect stress: 97284635 rounds by 64 tasks in 30 min, every answer as expected, rss 96 MB at the mark (median of minutes 15 to 17), 97 MB at the end (median of the last 3 minutes; 1.0%), peak 98 MB, pass, 30m0s
- llvm run 1: grpcreflect stress: 130990859 rounds by 64 tasks in 30 min, every answer as expected, rss 98 MB at the mark (median of minutes 15 to 17), 101 MB at the end (median of the last 3 minutes; 3.0%), peak 102 MB, pass, 30m0s
- llvm run 2: grpcreflect stress: 130494590 rounds by 64 tasks in 30 min, every answer as expected, rss 90 MB at the mark (median of minutes 15 to 17), 94 MB at the end (median of the last 3 minutes; 4.0%), peak 95 MB, pass, 30m0s
- arm64 --check=all: grpcreflect stress: 1880697 rounds by 64 tasks in 5 min, every answer as expected, rss not sampled, pass; xo leakcheck: 0 live of 690298031 allocations, 5m30s
