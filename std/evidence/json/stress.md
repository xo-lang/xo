kind: stress
module: json
result: pass
commit: b53462135
source: 57a1aa4d4abc6193c4bc0845d6619bd8f3fee2e3c5b0a9911ee742d760a72820
date: 2026-11-13
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64" json
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m6s

Stress evidence (decision 0616): the program std/evidence/json/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: json stress: 33044782400 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (3.0%), peak 6 MB, pass, 30m0s
- llvm: json stress: 31061233600 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 7 MB at the end (15.0%), peak 7 MB, pass, 30m0s
