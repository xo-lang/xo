kind: stress
module: otp
result: pass
commit: 32c89673e
source: 5ec3251979361e45f14dd056b95068bc275933f720f0aa53d7ed5275131c3fa3
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" otp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m25s

Stress evidence (decision 0616): the program std/evidence/otp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: otp stress: 316550571 keys by 64 tasks in 30 min, 0 identities broken, rss 13 MB at the mark, 13 MB at the end (4.0%), peak 13 MB, pass, 30m0s
- llvm: otp stress: 714934080 keys by 64 tasks in 30 min, 0 identities broken, rss 14 MB at the mark, 14 MB at the end (3.0%), peak 14 MB, pass, 30m0s
- arm64 --check=all: otp stress: 30179912 keys by 64 tasks in 5 min, 0 identities broken, rss 263 MB at the mark, 263 MB at the end (0.0%), peak 263 MB, pass; xo leakcheck: 0 live of 3759522500 allocations, 5m0s
