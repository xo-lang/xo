kind: stress
module: amqp
result: pass
commit: 719220f02
source: 2373df9a466fad4e4290590ae41d8c820e4ded471384c01e9bde38d9b77f38dd
date: 2026-11-17
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" amqp
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m15s

Stress evidence (decision 0616): the program std/evidence/amqp/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: amqp stress: 39893977 rounds by 64 tasks in 30 min, every message back as sent, rss 214 MB at the mark (median of minutes 5 to 7), 232 MB at the end (median of the last 3 minutes; 9.0%), peak 240 MB, pass, 30m0s
- llvm: amqp stress: 37942765 rounds by 64 tasks in 30 min, every message back as sent, rss 223 MB at the mark (median of minutes 5 to 7), 233 MB at the end (median of the last 3 minutes; 4.0%), peak 240 MB, pass, 30m0s
- arm64 --check=all: amqp stress: 879039 rounds by 64 tasks in 5 min, every message back as sent, rss not sampled, pass; xo leakcheck: 0 live of 260774007 allocations, 5m0s
