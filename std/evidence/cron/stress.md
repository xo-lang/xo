kind: stress
module: cron
result: pass
commit: 52efc3d15
source: 12b4ff0f4070fc871ad8adc11f03f5180e8b49b3127b6abb09ccf6f2365e5998
date: 2026-11-16
host: linux/arm64 host 4
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 4" cron
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 1h5m18s

Stress evidence (decision 0616): the program std/evidence/cron/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: cron stress: 14510006400 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark, 3 MB at the end (1.0%), peak 3 MB, pass, 30m0s
- llvm: cron stress: 11414453376 operations by 64 tasks in 30 min, 0 mismatches, rss 3 MB at the mark, 3 MB at the end (1.0%), peak 3 MB, pass, 30m0s
- arm64 --check=all: cron stress: 1314652672 operations by 64 tasks in 5 min, 0 mismatches, rss 71 MB at the mark, 71 MB at the end (0.0%), peak 71 MB, pass; xo leakcheck: 0 live of 16145579361 allocations, 5m0s
