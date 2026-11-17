kind: stress
module: udp
result: pass
commit: 97036298d
source: 7ecb96d6818c7cb5e9cfc5857e91592323fe38cb6afd8e04e7a88286294f3be0
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" udp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m31s

Stress evidence (decision 0616): the program std/evidence/udp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: udp stress: 249671311 datagrams echoed by 64 clients in 30 min, 37 dropped, 0 mismatches, rss 7 MB at the mark, 8 MB at the end (4.0%), peak 8 MB, pass, 30m0s
- llvm: udp stress: 221823235 datagrams echoed by 64 clients in 30 min, 2 dropped, 0 mismatches, rss 8 MB at the mark, 8 MB at the end (-10.0%), peak 9 MB, pass, 30m0s
- arm64 --check=all: udp stress: 3220084 datagrams echoed by 64 clients in 5 min, 0 dropped, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 28981138 allocations, 5m1s
