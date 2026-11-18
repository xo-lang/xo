kind: stress
module: grpc
result: pass
commit: a52ca820a
source: 2ce57edb5c6d1e326bf7164e7c52cf1d2dc4e2ec9f033d30e829bfa3ec49cbce
date: 2026-11-18
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 2" grpc
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m2s

Stress evidence (decision 0616): the program std/evidence/grpc/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: grpc stress: 9888026 rounds by 64 tasks in 30 min, every answer right, rss 293 MB at the mark (median of minutes 5 to 7), 298 MB at the end (median of the last 3 minutes; 2.0%), peak 308 MB, pass, 30m0s
- llvm: grpc stress: 9395262 rounds by 64 tasks in 30 min, every answer right, rss 307 MB at the mark (median of minutes 5 to 7), 309 MB at the end (median of the last 3 minutes; 0.0%), peak 321 MB, pass, 30m0s
- arm64 --check=all: grpc stress: 199773 rounds by 64 tasks in 5 min, every answer right, rss not sampled, pass; xo leakcheck: 0 live of 370416412 allocations, 5m0s
kind: stress
module: grpc
result: pass
commit: 8bf958ed8
source: 2ce57edb5c6d1e326bf7164e7c52cf1d2dc4e2ec9f033d30e829bfa3ec49cbce
date: 2026-11-18
host: linux/amd64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends amd64,llvm -check-minutes 5 -host "linux/amd64" grpc
measure: 64 tasks for 30 min on each of amd64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on amd64 under --check=all with 0 faults and 0 blocks live at exit, 1h7m28s

Stress evidence (decision 0616): the program std/evidence/grpc/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- amd64: grpc stress: 8064471 rounds by 64 tasks in 30 min, every answer right, rss 413 MB at the mark (median of minutes 5 to 7), 427 MB at the end (median of the last 3 minutes; 3.0%), peak 435 MB, pass, 30m0s
- llvm: grpc stress: 9952347 rounds by 64 tasks in 30 min, every answer right, rss 430 MB at the mark (median of minutes 5 to 7), 433 MB at the end (median of the last 3 minutes; 1.0%), peak 444 MB, pass, 30m0s
- amd64 --check=all: grpc stress: 240323 rounds by 64 tasks in 5 min, every answer right, rss not sampled, pass; xo leakcheck: 0 live of 445920658 allocations, 5m0s
