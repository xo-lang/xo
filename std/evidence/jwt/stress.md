kind: stress
module: jwt
result: pass
commit: 32c89673e
source: 57fb2b7fa238b2ead82805e2ee4a4850630d39bcf7484abd8c709f234bb24602
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" jwt
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m44s

Stress evidence (decision 0616): the program std/evidence/jwt/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: jwt stress: 19310967 tokens by 64 tasks in 30 min, 0 identities broken, rss 12 MB at the mark, 12 MB at the end (5.0%), peak 12 MB, pass, 30m0s
- llvm: jwt stress: 19898155 tokens by 64 tasks in 30 min, 0 identities broken, rss 11 MB at the mark, 12 MB at the end (5.0%), peak 12 MB, pass, 30m0s
- arm64 --check=all: jwt stress: 2066114 tokens by 64 tasks in 5 min, 0 identities broken, rss 409 MB at the mark, 410 MB at the end (0.0%), peak 410 MB, pass; xo leakcheck: 0 live of 1451459054 allocations, 5m1s
