kind: stress
module: websocket
result: pass
commit: e5e0b8d22
source: 4a9178912381aac54aab15dcb528013763c58a6063d4c2bcc3c443b4ffd3490c
date: 2026-11-19
host: linux/amd64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends amd64,llvm -check-minutes 5 -host "linux/amd64" websocket
measure: 64 tasks for 30 min on each of amd64,llvm, 0 mismatches, 0 faults, memory under 10% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717), 5 min on amd64 under --check=all with 0 faults and 0 blocks live at exit, 2h6m8s

Stress evidence (decision 0616): the program std/evidence/websocket/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- amd64 run 1: websocket stress: 301430548 messages echoed by 64 clients in 30 min, 0 mismatches, rss 166 MB at the mark (median of minutes 15 to 17), 176 MB at the end (median of the last 3 minutes; 6.0%), peak 179 MB, pass, 30m0s
- amd64 run 2: websocket stress: 293653967 messages echoed by 64 clients in 30 min, 0 mismatches, rss 165 MB at the mark (median of minutes 15 to 17), 165 MB at the end (median of the last 3 minutes; -0.0%), peak 172 MB, pass, 30m0s
- llvm run 1: websocket stress: 150192980 messages echoed by 64 clients in 30 min, 0 mismatches, rss 154 MB at the mark (median of minutes 15 to 17), 156 MB at the end (median of the last 3 minutes; 1.0%), peak 160 MB, pass, 30m1s
- llvm run 2: websocket stress: 150825123 messages echoed by 64 clients in 30 min, 0 mismatches, rss 150 MB at the mark (median of minutes 15 to 17), 156 MB at the end (median of the last 3 minutes; 4.0%), peak 160 MB, pass, 30m1s
- amd64 --check=all: websocket stress: 47781971 messages echoed by 64 clients in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 1044410965 allocations, 5m0s
