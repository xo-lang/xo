kind: stress
module: compress
result: pass
commit: 785612d4b
source: c6e28c6b292bd1f66102119f96c9c8a5f26d9474c9ea128482417979e5dba89d
date: 2026-11-14
host: linux/arm64 host 1
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -host "linux/arm64 host 1" compress
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 1h0m14s

Stress evidence (decision 0616): the program std/evidence/compress/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: compress stress: 90624600 operations by 64 tasks in 30 min, 0 mismatches, rss 91 MB at the mark, 93 MB at the end (2.0%), peak 97 MB, pass, 30m3s
- llvm: compress stress: 87238800 operations by 64 tasks in 30 min, 0 mismatches, rss 97 MB at the mark, 98 MB at the end (0.0%), peak 100 MB, pass, 30m3s
