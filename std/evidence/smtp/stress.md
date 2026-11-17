kind: stress
module: smtp
result: pass
commit: cdb4f4b9e
source: e2a32e3c5c154fa47ae5a336cbf024f327071461c8be0f98fb9e185e9447142c
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" smtp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m45s

Stress evidence (decision 0616): the program std/evidence/smtp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: smtp stress: 720064 sends by 64 clients in 30 min (17875 MB of bodies taken), 0 wrong results, rss 34 MB at the mark, 38 MB at the end (11.0%), peak 46 MB, pass, 30m0s
- llvm: smtp stress: 720064 sends by 64 clients in 30 min (17875 MB of bodies taken), 0 wrong results, rss 36 MB at the mark, 37 MB at the end (2.0%), peak 48 MB, pass, 30m0s
- arm64 --check=all: smtp stress: 120064 sends by 64 clients in 5 min (2981 MB of bodies taken), 0 wrong results, rss 534 MB at the mark, 457 MB at the end (-14.0%), peak 534 MB, pass; xo leakcheck: 0 live of 17867327 allocations, 5m0s
