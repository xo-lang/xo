kind: stress
module: hpke
result: pass
commit: 32c89673e
source: 5a65ccadfc5b5b84cce34abddbbaa8d759d7a3708a6642989a7f3940ed174d25
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" hpke
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m37s

Stress evidence (decision 0616): the program std/evidence/hpke/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: hpke stress: 13180335 keys by 64 tasks in 30 min, 0 identities broken, rss 16 MB at the mark, 17 MB at the end (4.0%), peak 17 MB, pass, 30m0s
- llvm: hpke stress: 15757109 keys by 64 tasks in 30 min, 0 identities broken, rss 17 MB at the mark, 17 MB at the end (3.0%), peak 17 MB, pass, 30m0s
- arm64 --check=all: hpke stress: 974702 keys by 64 tasks in 5 min, 0 identities broken, rss 868 MB at the mark, 869 MB at the end (0.0%), peak 869 MB, pass; xo leakcheck: 0 live of 504753557 allocations, 5m1s
