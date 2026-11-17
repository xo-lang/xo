kind: stress
module: asn1
result: pass
commit: 32c89673e
source: abe896b9ae3a2e7cd1c74b1a283abb5e967940a7d727d31127869c040a1d1494
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" asn1
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m24s

Stress evidence (decision 0616): the program std/evidence/asn1/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: asn1 stress: 5526611182 values by 64 tasks in 30 min, 0 identities broken, rss 8 MB at the mark, 9 MB at the end (3.0%), peak 9 MB, pass, 30m0s
- llvm: asn1 stress: 4660049625 values by 64 tasks in 30 min, 0 identities broken, rss 8 MB at the mark, 9 MB at the end (4.0%), peak 9 MB, pass, 30m0s
- arm64 --check=all: asn1 stress: 65536977 values by 64 tasks in 5 min, 0 identities broken, rss 237 MB at the mark, 239 MB at the end (1.0%), peak 239 MB, pass; xo leakcheck: 0 live of 1905317932 allocations, 5m0s
