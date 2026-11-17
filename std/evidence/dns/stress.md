kind: stress
module: dns
result: pass
commit: 9c3302527
source: 0e43ddc0b0d3efd0e3857790c905c11e973bbbc5205444009b1d3fafc6e73c8c
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" dns
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m28s

Stress evidence (decision 0616): the program std/evidence/dns/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: dns stress: 111948404 lookups by 64 clients in 30 min, 0 wrong answers, rss 14 MB at the mark, 16 MB at the end (21.0%), peak 16 MB, pass, 30m0s
- llvm: dns stress: 110802808 lookups by 64 clients in 30 min, 0 wrong answers, rss 13 MB at the mark, 17 MB at the end (25.0%), peak 17 MB, pass, 30m0s
- arm64 --check=all: dns stress: 2575351 lookups by 64 clients in 5 min, 0 wrong answers, rss 146 MB at the mark, 150 MB at the end (2.0%), peak 150 MB, pass; xo leakcheck: 0 live of 356327872 allocations, 5m0s
