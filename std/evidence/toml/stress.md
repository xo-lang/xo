kind: stress
module: toml
result: pass
commit: 0f6c58d77
source: 6c2fe6dd7e1b30c5beee7405191f3a940fa5b33d06de028a41cc3da3dc5a7dab
date: 2026-11-17
host: linux/arm64 host 3
command: go run ./scripts/stdproven stress -minutes 30 -tasks 64 -backends arm64,llvm -check-minutes 5 -host "linux/arm64 host 3" toml
measure: 64 tasks for 30 min on each of arm64,llvm, 0 mismatches, 0 faults, memory under 10% growth after 5 min, 5 min on arm64 under --check=all with 0 faults and 0 blocks live at exit, 30m43s

Stress evidence (decision 0616): the program std/evidence/toml/stress, every result checked against Go's answer, the resident set sampled every 10 s.

- arm64: toml stress: 1845660800 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 2 MB at the end (-40.0%), peak 4 MB, pass, 30m0s
- llvm: toml stress: 1990864800 operations by 64 tasks in 30 min, 0 mismatches, rss 4 MB at the mark, 2 MB at the end (-45.0%), peak 4 MB, pass, 30m0s
- arm64 --check=all: toml stress: 16455600 operations by 64 tasks in 5 min, 0 mismatches, rss not sampled, pass; xo leakcheck: 0 live of 878287940 allocations, 5m0s
