kind: stress
module: url
result: pass
commit: 719edcf26
source: 828c07e20050977f91ae4bedd1c8a18ed0bfc2a4e83f6bf20f743cf38fda169f
date: 2026-11-14
host: linux/arm64 host 2
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64 host 2" url
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m13s

Stress evidence (decision 0616): the program std/evidence/url/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: url stress: 159020160192 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
- llvm: url stress: 125618465088 operations by 64 tasks in 30 min, 0 mismatches, rss 5 MB at the mark, 5 MB at the end (1.0%), peak 5 MB, pass, 30m0s
