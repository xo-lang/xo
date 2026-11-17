kind: stress
module: yaml
result: pass
commit: 224654253
source: 51ad9c216133517a40a8a8eb75636f1f208e452ea95f474273b48352abd8ec63
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" yaml
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m18s

Stress evidence (decision 0616): the program std/evidence/yaml/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: yaml stress: 464901120 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 7 MB at the end (2.0%), peak 7 MB, pass, 30m0s
- llvm: yaml stress: 502391296 operations by 64 tasks in 30 min, 0 mismatches, rss 8 MB at the mark, 8 MB at the end (2.0%), peak 8 MB, pass, 30m0s
- arm64 --check=all: yaml stress: 12025344 operations by 64 tasks in 5 min, 0 mismatches, rss 307 MB at the mark, 307 MB at the end (-0.0%), peak 307 MB, pass; xo leakcheck: 0 live of 775643815 allocations, 5m1s
