kind: stress
module: cookiejar
result: pass
commit: 32c89673e
source: 049f0da5d7a869d74a1bb8c293768496bb3ccea03896f0243029fcd88749a13e
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" cookiejar
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m28s

Stress evidence (decision 0616): the program std/evidence/cookiejar/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: cookiejar stress: 302098502 rounds by 64 tasks in 30 min, 0 identities broken, rss 4 MB at the mark, 5 MB at the end (31.0%), peak 5 MB, pass, 30m0s
- llvm: cookiejar stress: 366470806 rounds by 64 tasks in 30 min, 0 identities broken, rss 3 MB at the mark, 5 MB at the end (57.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: cookiejar stress: 17522323 rounds by 64 tasks in 5 min, 0 identities broken, rss 321 MB at the mark, 346 MB at the end (8.0%), peak 346 MB, pass; xo leakcheck: 0 live of 554874578 allocations, 5m0s
