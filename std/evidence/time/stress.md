kind: stress
module: time
result: pass
commit: a0c61aef6
source: 615bb2e80abc81996374c87057a298c015893ddcb4f2d2118bbf142de8e94fba
date: 2026-11-14
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64" time
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m8s

Stress evidence (decision 0616): the program std/evidence/time/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: time stress: 204358310400 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: time stress: 189062099968 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 2 MB at the end (-58.0%), peak 5 MB, pass, 30m0s
