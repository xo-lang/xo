kind: stress
module: tls
result: pass
commit: ef0cb20d5
source: 94f51ddc28fe734ccdfbd9f5ceaf04e80513aa6725a74cb1712e440df4d8e70b
date: 2026-11-18
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" tls
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h4m14s

Stress evidence (decision 0616): the program std/evidence/tls/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64 run 1: tls stress: 385905 connections (192934 resumed), 29849 MiB echoed by 64 clients in 30 min, 0 mismatches, rss 57 MB at the mark (median of minutes 15 to 17), 57 MB at the end (median of the last 3 minutes; -1.0%), peak 66 MB, pass, 30m0s
- arm64 run 2: tls stress: 392855 connections (196411 resumed), 30608 MiB echoed by 64 clients in 30 min, 0 mismatches, rss 58 MB at the mark (median of minutes 15 to 17), 58 MB at the end (median of the last 3 minutes; 1.0%), peak 72 MB, pass, 30m0s
- llvm run 1: tls stress: 384149 connections (192057 resumed), 29655 MiB echoed by 64 clients in 30 min, 0 mismatches, rss 64 MB at the mark (median of minutes 15 to 17), 69 MB at the end (median of the last 3 minutes; 6.0%), peak 72 MB, pass, 30m0s
- llvm run 2: tls stress: 398375 connections (199173 resumed), 31172 MiB echoed by 64 clients in 30 min, 0 mismatches, rss 63 MB at the mark (median of minutes 15 to 17), 63 MB at the end (median of the last 3 minutes; -1.0%), peak 69 MB, pass, 30m0s
- arm64 --check=all: tls stress: 63076 connections (31522 resumed), 4485 MiB echoed by 64 clients in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 122080913 allocations, 5m0s
