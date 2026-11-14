kind: stress
module: http
result: pass
commit: 85692d8af
source: 66883afcd9020c1f92253ea7b9bd3bb299a4eed8a893c2b395102e775ea707d1
date: 2026-11-14
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64 host 2" http
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m54s

Stress evidence (decision 0616): the program std/evidence/http/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: http stress: 328080624 requests by 64 clients and 26798 hostile connections by 8 in 30 min, 0 mismatches, rss 78 MB at the mark, 72 MB at the end (-7.0%), peak 80 MB, pass, 30m2s
- llvm: http stress: 390462120 requests by 64 clients and 27182 hostile connections by 8 in 30 min, 0 mismatches, rss 82 MB at the mark, 88 MB at the end (8.0%), peak 90 MB, pass, 30m1s
