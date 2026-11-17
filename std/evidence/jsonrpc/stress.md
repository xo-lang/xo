kind: stress
module: jsonrpc
result: pass
commit: fbda2c389
source: 94fc70f7cd36742ed9c8fb925d38313693152327424cc68580f6d71d558482a7
date: 2026-11-17
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" jsonrpc
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m10s

Stress evidence (decision 0616): the program std/evidence/jsonrpc/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: jsonrpc stress: 78095197 calls by 128 tasks in 30 min, 4880949 notifications, every answer right, rss 98 MB at the mark (median of minutes 5 to 7), 99 MB at the end (median of the last 3 minutes; 1.0%), peak 112 MB, pass, 30m0s
- llvm: jsonrpc stress: 97942785 calls by 128 tasks in 30 min, 6121412 notifications, every answer right, rss 103 MB at the mark (median of minutes 5 to 7), 102 MB at the end (median of the last 3 minutes; -1.0%), peak 112 MB, pass, 30m0s
- arm64 --check=all: jsonrpc stress: 1463359 calls by 128 tasks in 5 min, 91452 notifications, every answer right, rss not sampled, pass; xo leakcheck: 0 live of 664659508 allocations, 5m0s
