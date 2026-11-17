kind: stress
module: textproto
result: pass
commit: 224654253
source: 3ee6c206038dafad864801f91bf39b968b19e5e2a161726cd9d33bc0552b0da8
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" textproto
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 31m28s

Stress evidence (decision 0616): the program std/evidence/textproto/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: textproto stress: 1815024310 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 4 MB at the end (1.0%), peak 4 MB, pass, 30m0s
- llvm: textproto stress: 1953127076 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- arm64 --check=all: textproto stress: 59664051 operations by 64 tasks in 5 min, 0 mismatches, rss 111 MB at the mark, 111 MB at the end (0.0%), peak 111 MB, pass; xo leakcheck: 0 live of 4045046197 allocations, 5m0s
