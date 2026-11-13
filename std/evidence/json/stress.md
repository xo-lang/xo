kind: stress
module: json
result: pass
commit: 15811f919
source: d489d07c0378b0289a0bc3bafcda807e108ed06a84a62ad880ad8d5dec7dda29
date: 2026-11-13
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64 host 2" json
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m10s

Stress evidence (decision 0616): the program std/evidence/json/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: json stress: 27112392960 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 7 MB at the end (2.0%), peak 7 MB, pass, 30m0s
- llvm: json stress: 26905160000 operations by 64 tasks in 30 min, 0 mismatches, rss 7 MB at the mark, 7 MB at the end (2.0%), peak 7 MB, pass, 30m0s
