kind: stress
module: time
result: pass
commit: 4c2c133dd
source: b179c89e3ae58e0d68d5883cdb26c3b8a7f2a8b8c5ac3ea5e505915154dfd398
date: 2026-11-14
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64" time
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m7s

Stress evidence (decision 0616): the program std/evidence/time/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: time stress: 194861488640 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: time stress: 206548847104 operations by 64 tasks in 30 min, 0 mismatches, rss 6 MB at the mark, 6 MB at the end (1.0%), peak 6 MB, pass, 30m0s
