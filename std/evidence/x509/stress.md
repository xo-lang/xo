kind: stress
module: x509
result: pass
commit: b9747bc5f
source: b5e2af4c9a964fcc73d6ee82e930f56d7ded1223e51bbba26f87d5c9f722aea2
date: 2026-11-13
host: linux/arm64
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64" x509
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m12s

Stress evidence (decision 0616): the program std/evidence/x509/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: x509 stress: 375574912 operations by 64 tasks in 30 min, 0 mismatches, rss 8 MB at the mark, 8 MB at the end (2.0%), peak 8 MB, pass, 30m0s
- llvm: x509 stress: 425415618 operations by 64 tasks in 30 min, 0 mismatches, rss 9 MB at the mark, 9 MB at the end (0.0%), peak 9 MB, pass, 30m0s
