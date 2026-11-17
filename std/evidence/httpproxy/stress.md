kind: stress
module: httpproxy
result: pass
commit: 1340eb974
source: ca55b3fa3f47c9c8b901c8bd66442d664178be471d3509bb667adf9f1caca3c5
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" httpproxy
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m19s

Stress evidence (decision 0616): the program std/evidence/httpproxy/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: httpproxy stress: 8898201600 lookups by 64 tasks in 30 min over 64 configurations (11721 of 16384 answers a proxy), every answer as recorded, rss 5 MB at the mark, 5 MB at the end (0.0%), peak 5 MB, pass, 30m0s
- llvm: httpproxy stress: 9535234048 lookups by 64 tasks in 30 min over 64 configurations (11721 of 16384 answers a proxy), every answer as recorded, rss 5 MB at the mark, 5 MB at the end (2.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: httpproxy stress: 1139124224 lookups by 64 tasks in 5 min over 64 configurations (11721 of 16384 answers a proxy), every answer as recorded, rss 31 MB at the mark, 32 MB at the end (0.0%), peak 32 MB, pass; xo leakcheck: 0 live of 3700137655 allocations, 5m0s
